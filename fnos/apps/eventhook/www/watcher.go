package main

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// copy DB to temp file to avoid SQLITE_BUSY with eventlogger service
func openEventLoggerDB(srcPath string) (*sql.DB, func(), error) {
	tmpPath := "/tmp/eventhook-cache.db3"
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("copy db: %w", err)
	}
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return nil, nil, fmt.Errorf("write tmp db: %w", err)
	}
	dsn := tmpPath + "?_pragma=query_only(true)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		os.Remove(tmpPath)
		return nil, nil, err
	}
	cleanup := func() {
		db.Close()
		os.Remove(tmpPath)
	}
	return db, cleanup, nil
}

type Watcher struct {
	mu        sync.RWMutex
	running   bool
	stopCh    chan struct{}
	cursor    int64
	lastError string
}

func (w *Watcher) getLastError() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastError
}

func (w *Watcher) setLastError(err string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastError = err
}

func newWatcher() *Watcher {
	return &Watcher{stopCh: make(chan struct{})}
}

func (w *Watcher) isRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

func (w *Watcher) getCursor() int64 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.cursor
}

func (w *Watcher) loadCursor() {
	var id int64
	err := db.QueryRow("SELECT COALESCE(MAX(event_id),0) FROM cursor_pos").Scan(&id)
	if err != nil {
		w.cursor = 0
		return
	}
	w.cursor = id
}

func (w *Watcher) saveCursor(id int64) {
	_, _ = db.Exec("DELETE FROM cursor_pos")
	_, _ = db.Exec("INSERT INTO cursor_pos(event_id) VALUES(?)", id)
}

func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	w.loadCursor()
	appLogf("watcher started, cursor=%d", w.cursor)
	go w.loop()
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		return
	}
	w.running = false
	close(w.stopCh)
	appLogf("watcher stopped")
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.stopCh:
			return
		case <-time.After(time.Duration(settings.PollInterval) * time.Second):
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	if settings.EventloggerDB == "" {
		w.setLastError("未配置 EventLogger 数据库路径")
		return
	}

	dsn := settings.EventloggerDB + "?_pragma=query_only(true)&_pragma=busy_timeout(5000)"
	eventDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		errMsg := "打开 EventLogger 数据库失败: " + err.Error()
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}
	defer eventDB.Close()

	if err := eventDB.Ping(); err != nil {
		errMsg := "连接 EventLogger 数据库失败: " + err.Error()
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}

	query, err := buildEventQuery(eventDB)
	if err != nil {
		errMsg := "检测 EventLogger 表结构失败: " + err.Error()
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}

	rows, err := eventDB.Query(query, w.cursor)
	if err != nil {
		errMsg := "查询 EventLogger 失败: " + err.Error()
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}
	defer rows.Close()

	colNames, _ := rows.Columns()
	if len(colNames) == 0 {
		w.setLastError("EventLogger 表无可用列")
		return
	}
	colMap := make(map[string]int)
	for i, c := range colNames {
			colMap[strings.ToLower(c)] = i
	}

	idIdx := -1
	if idx, ok := colMap["id"]; ok {
		idIdx = idx
	} else if idx, ok := colMap["event_id"]; ok {
		idIdx = idx
	} else if idx, ok := colMap["_rid"]; ok {
		idIdx = idx
	}
	if idIdx < 0 {
		errMsg := fmt.Sprintf("EventLogger 表无 id/event_id 列 (可用列: %v)", colNames)
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}

	typeIdx := -1
	if idx, ok := colMap["type"]; ok {
		typeIdx = idx
	} else if idx, ok := colMap["event"]; ok {
		typeIdx = idx
	} else if idx, ok := colMap["event_type"]; ok {
		typeIdx = idx
	} else if idx, ok := colMap["eventid"]; ok {
		typeIdx = idx
	} else if idx, ok := colMap["serviceid"]; ok {
		typeIdx = idx
	} else if idx, ok := colMap["category"]; ok {
		typeIdx = idx
	}
	detailIdx := -1
	if idx, ok := colMap["detail"]; ok {
		detailIdx = idx
	} else if idx, ok := colMap["content"]; ok {
		detailIdx = idx
	} else if idx, ok := colMap["msg"]; ok {
		detailIdx = idx
	} else if idx, ok := colMap["message"]; ok {
		detailIdx = idx
	} else if idx, ok := colMap["parameter"]; ok {
		detailIdx = idx
	}
	tsIdx := -1
	if idx, ok := colMap["timestamp"]; ok {
		tsIdx = idx
	} else if idx, ok := colMap["time"]; ok {
		tsIdx = idx
	} else if idx, ok := colMap["created_at"]; ok {
		tsIdx = idx
	} else if idx, ok := colMap["create_time"]; ok {
		tsIdx = idx
	} else if idx, ok := colMap["logtime"]; ok {
		tsIdx = idx
	}

	if typeIdx < 0 {
		errMsg := fmt.Sprintf("EventLogger 表无 type/event 列 (可用列: %v)", colNames)
		appLogf(errMsg)
		w.setLastError(errMsg)
		return
	}

	// poll succeeded — clear any previous error
	w.setLastError("")

	for rows.Next() {
		values := make([]any, len(colNames))
		ptrs := make([]any, len(colNames))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			appLogf("scan row: %v", err)
			continue
		}

		var id int64
		if idIdx >= 0 && values[idIdx] != nil {
			switch v := values[idIdx].(type) {
			case int64:
				id = v
			case float64:
				id = int64(v)
			case int:
				id = int64(v)
			case int32:
				id = int64(v)
			case []byte:
				id, _ = strconv.ParseInt(string(v), 10, 64)
			case string:
				id, _ = strconv.ParseInt(v, 10, 64)
			}
		}
		if id <= w.cursor {
			continue
		}

		etype := fmt.Sprintf("%v", values[typeIdx])
		detail := ""
		if detailIdx >= 0 && values[detailIdx] != nil {
			detail = fmt.Sprintf("%v", values[detailIdx])
		}
		ts := ""
		if tsIdx >= 0 && values[tsIdx] != nil {
			switch v := values[tsIdx].(type) {
			case int64:
				ts = time.Unix(v, 0).Format("2006-01-02 15:04:05")
			case float64:
				ts = time.Unix(int64(v), 0).Format("2006-01-02 15:04:05")
			case string:
				ts = v
			}
		}

		w.processEvent(id, etype, detail, ts)
		w.cursor = id
		w.saveCursor(id)
	}
}

type tableInfo struct {
	name string
	idCol string
}

var knownTables = []string{"event_log", "log", "events", "syslog", "event_logger", "t_log", "system_log"}

func buildEventQuery(eventDB *sql.DB) (string, error) {
	// 1. First try known table names in priority order
	for _, t := range knownTables {
		idCol := detectIDColumn(eventDB, t)
		if idCol != "" {
			return fmt.Sprintf("SELECT * FROM \"%s\" WHERE \"%s\" > ? ORDER BY \"%s\" ASC LIMIT 100", t, idCol, idCol), nil
		}
	}
	// 2. Fallback: try ANY table with an id-like column
	rows, err := eventDB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		idCol := detectIDColumn(eventDB, name)
		if idCol != "" {
			return fmt.Sprintf("SELECT * FROM \"%s\" WHERE \"%s\" > ? ORDER BY \"%s\" ASC LIMIT 100", name, idCol, idCol), nil
		}
		// 3. Try rowid (every SQLite table has one) - must select it explicitly
		return fmt.Sprintf("SELECT *, rowid AS _rid FROM \"%s\" WHERE rowid > ? ORDER BY rowid ASC LIMIT 100", name), nil
	}
	return "", fmt.Errorf("no table found in eventlogger db")
}

func detectIDColumn(eventDB *sql.DB, table string) string {
	rows, err := eventDB.Query(fmt.Sprintf("PRAGMA table_info(\"%s\")", table))
	if err != nil {
		return ""
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "id" || lower == "event_id" || lower == "eid" || lower == "_id" {
			return name
		}
	}
	return ""
}

var dedupMu sync.Mutex
var dedupMap = make(map[string]time.Time)

func (w *Watcher) processEvent(id int64, etype, detail, ts string) {
	if ts == "" {
		ts = time.Now().Format("2006-01-02 15:04:05")
	}

	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = etype
	}

	appLogf("event: id=%d type=%s detail=%s", id, etype, detail)

	settingsMu.RLock()
	dndStart := settings.DndStart
	dndEnd := settings.DndEnd
	dedupWindow := settings.DedupWindow
	settingsMu.RUnlock()

	if dedupWindow > 0 {
		dedupMu.Lock()
		if last, ok := dedupMap[etype]; ok && time.Since(last) < time.Duration(dedupWindow)*time.Second {
			dedupMu.Unlock()
			appLogf("  -> dedup skipped (type=%s)", etype)
			saveEventRecord(id, etype, detail, ResultSkipped, "dedup", "")
			return
		}
		dedupMap[etype] = time.Now()
		dedupMu.Unlock()
	}

	now := time.Now()
	isDND := false
	if dndStart != "" && dndEnd != "" {
		startParts := strings.Split(dndStart, ":")
		endParts := strings.Split(dndEnd, ":")
		if len(startParts) == 2 && len(endParts) == 2 {
			startH := parseInt(startParts[0], 0)
			startM := parseInt(startParts[1], 0)
			endH := parseInt(endParts[0], 0)
			endM := parseInt(endParts[1], 0)
			nowMinutes := now.Hour()*60 + now.Minute()
			startMinutes := startH*60 + startM
			endMinutes := endH*60 + endM
			if startMinutes <= endMinutes {
				isDND = nowMinutes >= startMinutes && nowMinutes < endMinutes
			} else {
				isDND = nowMinutes >= startMinutes || nowMinutes < endMinutes
			}
		}
	}

	if isDND {
		appLogf("  -> dnd skipped (type=%s)", etype)
		saveEventRecord(id, etype, detail, ResultSkipped, "dnd", "")
		return
	}

	hooksMu.RLock()
	var matchedHooks []Hook
	for _, h := range hooks {
		if !h.Enabled {
			continue
		}
		etypeUpper := strings.ToUpper(etype)
		for _, et := range h.EventTypes {
			if et == "*" || strings.ToUpper(et) == etypeUpper {
				matchedHooks = append(matchedHooks, h)
				break
			}
		}
	}
	hooksMu.RUnlock()

	if len(matchedHooks) == 0 {
		hookInfo := ""
		for _, h := range hooks {
			if h.Enabled {
				hookInfo += fmt.Sprintf(" [%s: %v]", h.Name, h.EventTypes)
			}
		}
		appLogf("  -> no matching hooks (type=%s, active_hooks:%s)", etype, hookInfo)
		saveEventRecord(id, etype, detail, ResultSkipped, "no_hook", "")
		return
	}

	for _, h := range matchedHooks {
		err := executeHook(h, etype, detail, ts)
		result := ResultSent
		errMsg := ""
		if err != nil {
			result = ResultFailed
			errMsg = err.Error()
		}
		appLogf("  -> hook %s (id=%d type=%s): %s", h.Name, h.ID, h.Type, result)
		saveEventRecord(id, etype, detail, result, h.Name, errMsg)
	}
}

func saveEventRecord(eventID int64, etype, detail, result, hookName, errMsg string) {
	_, _ = db.Exec(
		"INSERT INTO event_log(event_id, type, detail, result, hook_name, error, created_at) VALUES(?,?,?,?,?,?,datetime('now','localtime'))",
		eventID, etype, detail, result, hookName, errMsg,
	)
}

func parseInt(s string, def int) int {
	var n int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	if s == "" {
		return def
	}
	return n
}
