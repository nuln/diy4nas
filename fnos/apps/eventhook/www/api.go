package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	hooksMu sync.RWMutex
	hooks   []Hook
)

func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/healthz", handleHealthz)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/hooks", handleHooks)
	mux.HandleFunc("/api/hooks/", handleHookByID)
	mux.HandleFunc("/api/events", handleEvents)
	mux.HandleFunc("/api/settings", handleSettings)
	mux.HandleFunc("/api/stats", handleStats)
	mux.HandleFunc("/api/log", handleLog)
	mux.HandleFunc("/", handleIndex)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "frontend missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{
		"ok":     true,
		"time":   timeNow(),
		"watcher": watcher.isRunning(),
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	hi := HealthInfo{
		OK:           true,
		Time:         timeNow(),
		Watcher:      watcher.isRunning(),
		WatcherError: watcher.getLastError(),
		Cursor:       watcher.getCursor(),
		DBPath:       settings.EventloggerDB,
		DBReachable:  false,
		DBTables:     nil,
	}
	db.QueryRow("SELECT COUNT(*) FROM event_log").Scan(&hi.TotalEvents)
	hooksMu.RLock()
	hi.HooksCount = len(hooks)
	hooksMu.RUnlock()

	if settings.EventloggerDB != "" {
		dsn := settings.EventloggerDB + "?_pragma=query_only(true)&_pragma=busy_timeout(5000)"
		eventDB, err := sql.Open("sqlite", dsn)
		if err != nil {
			hi.DBError = "open: " + err.Error()
		} else {
			defer eventDB.Close()
			if err := eventDB.Ping(); err != nil {
				hi.DBError = "ping: " + err.Error()
			} else {
				hi.DBReachable = true
				rows, qerr := eventDB.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
				if qerr != nil {
					hi.DBError = "query: " + qerr.Error()
				} else if rows != nil {
					defer rows.Close()
					for rows.Next() {
						var name string
						rows.Scan(&name)
						hi.DBTables = append(hi.DBTables, name)
					}
				}
			}
		}
	}
	if hi.DBTables == nil {
		hi.DBTables = []string{}
	}
	jsonResponse(w, 200, hi)
}

func handleHooks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hooksMu.RLock()
		list := hooks
		hooksMu.RUnlock()
		if list == nil {
			list = []Hook{}
		}
		jsonResponse(w, 200, list)
	case http.MethodPost:
		var in HookInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, 400, "invalid body")
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			jsonError(w, 400, "name required")
			return
		}
		if len(in.EventTypes) == 0 {
			jsonError(w, 400, "event_types required")
			return
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		now := timeNow()
		result, err := db.Exec(
			"INSERT INTO hooks(name, type, enabled, url, token, cmd, headers, event_types, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)",
			in.Name, in.Type, boolToInt(enabled), in.URL, in.Token, in.Cmd, in.Headers, strings.Join(in.EventTypes, ","), now, now,
		)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		id, _ := result.LastInsertId()
		loadHooks()
		hook := findHook(id)
		if hook == nil {
			jsonError(w, 500, "hook not found after create")
			return
		}
		jsonResponse(w, 201, hook)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleHookByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/hooks/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, 400, "missing id")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, 400, "invalid id")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		h := findHook(id)
		if h == nil {
			jsonError(w, 404, "hook not found")
			return
		}
		jsonResponse(w, 200, h)
	case action == "" && r.Method == http.MethodPut:
		var in HookInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, 400, "invalid body")
			return
		}
		hooksMu.RLock()
		existing := findHook(id)
		hooksMu.RUnlock()
		if existing == nil {
			jsonError(w, 404, "hook not found")
			return
		}
		name := existing.Name
		if in.Name != "" {
			name = in.Name
		}
		typ := existing.Type
		if in.Type != "" {
			typ = in.Type
		}
		enabled := existing.Enabled
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		url := existing.URL
		if in.URL != "" {
			url = in.URL
		}
		token := existing.Token
		if in.Token != "" {
			token = in.Token
		}
		cmd := existing.Cmd
		if in.Cmd != "" {
			cmd = in.Cmd
		}
		headers := existing.Headers
		if in.Headers != "" {
			headers = in.Headers
		}
		eventTypes := strings.Join(existing.EventTypes, ",")
		if len(in.EventTypes) > 0 {
			eventTypes = strings.Join(in.EventTypes, ",")
		}
		now := timeNow()
		_, err := db.Exec(
			"UPDATE hooks SET name=?, type=?, enabled=?, url=?, token=?, cmd=?, headers=?, event_types=?, updated_at=? WHERE id=?",
			name, typ, boolToInt(enabled), url, token, cmd, headers, eventTypes, now, id,
		)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		loadHooks()
		h := findHook(id)
		if h == nil {
			jsonError(w, 404, "hook not found after update")
			return
		}
		jsonResponse(w, 200, h)
	case action == "" && r.Method == http.MethodDelete:
		_, err := db.Exec("DELETE FROM hooks WHERE id=?", id)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		loadHooks()
		jsonResponse(w, 200, map[string]any{"ok": true})
	case action == "toggle" && r.Method == http.MethodPost:
		h := findHook(id)
		if h == nil {
			jsonError(w, 404, "hook not found")
			return
		}
		newVal := boolToInt(!h.Enabled)
		_, err := db.Exec("UPDATE hooks SET enabled=? WHERE id=?", newVal, id)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		loadHooks()
		jsonResponse(w, 200, map[string]any{"enabled": !h.Enabled})
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 200)
	typeFilter := r.URL.Query().Get("type")
	resultFilter := r.URL.Query().Get("result")

	query := "SELECT id, event_id, type, detail, result, hook_name, error, created_at FROM event_log WHERE 1=1"
	var args []any

	if typeFilter != "" {
		query += " AND LOWER(type)=LOWER(?)"
		args = append(args, typeFilter)
	}
	if resultFilter != "" {
		query += " AND result=?"
		args = append(args, resultFilter)
	}
	query += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	defer rows.Close()

	events := make([]EventRecord, 0)
	for rows.Next() {
		var e EventRecord
		var eid int64
		var hook, errMsg string
		if err := rows.Scan(&e.ID, &eid, &e.Type, &e.Detail, &e.Result, &hook, &errMsg, &e.Timestamp); err != nil {
			continue
		}
		e.HookName = hook
		e.Error = errMsg
		events = append(events, e)
	}
	if events == nil {
		events = []EventRecord{}
	}
	jsonResponse(w, 200, events)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settingsMu.RLock()
		s := settings
		settingsMu.RUnlock()
		jsonResponse(w, 200, s)
	case http.MethodPut, http.MethodPost:
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, 400, "invalid body")
			return
		}
		settingsMu.Lock()
		if in.PollInterval > 0 {
			settings.PollInterval = in.PollInterval
		}
		if in.DedupWindow >= 0 {
			settings.DedupWindow = in.DedupWindow
		}
		settings.DndStart = in.DndStart
		settings.DndEnd = in.DndEnd
		if in.EventloggerDB != "" {
			if settings.EventloggerDB != in.EventloggerDB {
				settings.EventloggerDB = in.EventloggerDB
			}
		}
		s := settings
		settingsMu.Unlock()
		if err := saveSettingsFile(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, s)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	var total, sent, skipped int
	db.QueryRow("SELECT COUNT(*) FROM event_log").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM event_log WHERE result=?", ResultSent).Scan(&sent)
	db.QueryRow("SELECT COUNT(*) FROM event_log WHERE result=?", ResultSkipped).Scan(&skipped)

	hooksMu.RLock()
	totalHooks := len(hooks)
	hooksMu.RUnlock()

	rows, _ := db.Query("SELECT id, event_id, type, detail, result, hook_name, error, created_at FROM event_log ORDER BY id DESC LIMIT 10")
	recent := make([]EventRecord, 0)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var e EventRecord
			var eid int64
			var hook, errMsg string
			if err := rows.Scan(&e.ID, &eid, &e.Type, &e.Detail, &e.Result, &hook, &errMsg, &e.Timestamp); err == nil {
				e.HookName = hook
				recent = append(recent, e)
			}
		}
	}

	stats := Stats{
		TotalEvents:   total,
		SentEvents:    sent,
		SkippedEvents: skipped,
		TotalHooks:    totalHooks,
		WatcherRunning: watcher.isRunning(),
		RecentEvents:  recent,
	}
	jsonResponse(w, 200, stats)
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	lines := parseIntDefault(r.URL.Query().Get("lines"), 200)
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	all := strings.Split(string(data), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(strings.Join(all, "\n")))
}

func loadHooks() {
	rows, err := db.Query("SELECT id, name, type, enabled, url, token, cmd, headers, event_types, created_at, updated_at FROM hooks ORDER BY id")
	if err != nil {
		appLogf("load hooks: %v", err)
		return
	}
	defer rows.Close()

	hooksMu.Lock()
	defer hooksMu.Unlock()
	hooks = make([]Hook, 0)
	for rows.Next() {
		var h Hook
		var eventTypesStr string
		if err := rows.Scan(&h.ID, &h.Name, &h.Type, &h.Enabled, &h.URL, &h.Token, &h.Cmd, &h.Headers, &eventTypesStr, &h.CreatedAt, &h.UpdatedAt); err != nil {
			continue
		}
		if eventTypesStr != "" {
			h.EventTypes = strings.Split(eventTypesStr, ",")
		} else {
			h.EventTypes = []string{}
		}
		hooks = append(hooks, h)
	}
	appLogf("loaded %d hooks", len(hooks))
}

func findHook(id int64) *Hook {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	for _, h := range hooks {
		if h.ID == id {
			return &h
		}
	}
	return nil
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

var _ = fmt.Sprintf
