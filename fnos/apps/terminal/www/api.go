package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func registerRoutes(mux *http.ServeMux) {
	api := http.NewServeMux()
	api.HandleFunc("/api/healthz", handleHealthz)
	api.HandleFunc("/api/sessions", handleSessions)
	api.HandleFunc("/api/sessions/", handleSessionByID)
	api.HandleFunc("/api/ws", handleWS)
	api.HandleFunc("/api/settings", handleSettings)
	api.HandleFunc("/api/scripts", handleScripts)
	api.HandleFunc("/api/scripts/", handleScriptByID)
	mux.Handle("/api/", api)
	// 静态资源（vendor/xterm.js 等）必须先于 / 通配匹配
	mux.HandleFunc("/vendor/", handleStatic)
	mux.HandleFunc("/", handleIndex)
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	// 只允许 /vendor/ 路径，且文件名必须在白名单内（防目录穿越）
	upath := r.URL.Path
	if !strings.HasPrefix(upath, "/vendor/") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(upath, "/vendor/")
	// 防止 ../ 绕过
	if strings.Contains(name, "..") || strings.Contains(name, "//") {
		http.NotFound(w, r)
		return
	}
	fp := "ui/vendor/" + name
	data, err := frontend.ReadFile(fp)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// 根据文件扩展名设 Content-Type
	ct := "application/octet-stream"
	switch {
	case strings.HasSuffix(name, ".js"):
		ct = "application/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		ct = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(data)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/app/") {
		http.NotFound(w, r)
		return
	}
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "frontend missing", http.StatusInternalServerError)
		return
	}
	// 把当前用户信息注入到 HTML 里（index.html 会用 _USER/_ALL_USERS 变量显示）
	// 拿不到用户也照样 render（让 UI 提示错误）
	user, userErr := resolveRequestUser(r)
	allUsers := listSystemUsers()
	errStr := ""
	if userErr != nil {
		errStr = userErr.Error()
	}
	// 用 <!--FNOS_USER::...-->...<!--FNOS_USER_END--> 标记替换
	script := "<script>window._FNOS_USER=" + jsonString(user) +
		";window._FNOS_USER_ERROR=" + jsonString(errStr) +
		";window._FNOS_ALL_USERS=" + jsonString(allUsers) + ";</script>"
	data = []byte(strings.Replace(string(data), "</head>", script+"</head>", 1))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"ok":            true,
		"time":          time.Now().Format("2006-01-02 15:04:05"),
		"default_shell": settings.DefaultShell,
	})
}

func handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		includeDetached := r.URL.Query().Get("include_detached") == "true"
		writeJSON(w, 200, listSessions(includeDetached))
	case http.MethodPost:
		var in SessionCreate
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, 400, "invalid body")
			return
		}
		settingsMu.RLock()
		maxN := settings.MaxSessions
		settingsMu.RUnlock()
		sessionsMu.RLock()
		activeCount := 0
		for _, s := range sessions {
			s.mu.Lock()
			if !s.exited {
				activeCount++
			}
			s.mu.Unlock()
		}
		sessionsMu.RUnlock()
		if activeCount >= maxN {
			appLogf("create session blocked: active=%d maxN=%d", activeCount, maxN)
			writeErr(w, 429, "session limit reached")
			return
		}
		if in.Shell == "" {
			// 优先用 user 的登录 shell（从 /etc/passwd 读），支持 zsh/ohmyzsh
			// 如果 user 没指定 / 查不到，fallback 到 settings.DefaultShell
			userName := in.User
			if userName == "" {
				userName, _ = resolveRequestUser(r)
			}
			if u, err := lookupUser(userName); err == nil && u.sh != "" {
				in.Shell = u.sh
			} else {
				settingsMu.RLock()
				in.Shell = settings.DefaultShell
				settingsMu.RUnlock()
			}
		}
		// 优先级：前端指定 > 当前请求用户 (fnOS 反代 X-Forwarded-User)
		// 都不存在则报错（不再 fallback）
		user := in.User
		if user == "" {
			var err error
			user, err = resolveRequestUser(r)
			if err != nil {
				writeErr(w, 401, err.Error())
				return
			}
		}
		s, err := createSession(in, user)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, s.info())
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func handleSessionByID(w http.ResponseWriter, r *http.Request) {
	// 支持嵌套 action: /api/sessions/{id}/kill, /api/sessions/{id}/detach
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if rest == "" {
		writeErr(w, 400, "missing id")
		return
	}
	// 特殊路由: /api/sessions/kill-all
	if rest == "kill-all" {
		if r.Method != http.MethodPost {
			writeErr(w, 405, "method not allowed")
			return
		}
		// 杀所有 sessions
		sessionsMu.Lock()
		all := make([]*Session, 0, len(sessions))
		for _, s := range sessions {
			all = append(all, s)
		}
		sessions = make(map[string]*Session)
		sessionsMu.Unlock()
		for _, s := range all {
			s.close()
		}
		writeJSON(w, 200, map[string]any{"ok": true, "killed": len(all)})
		return
	}
	// 切分 id + action
	var id, action string
	if idx := strings.Index(rest, "/"); idx >= 0 {
		id = rest[:idx]
		action = rest[idx+1:]
	} else {
		id = rest
	}
	s := getSession(id)
	if s == nil {
		writeErr(w, 404, "session not found")
		return
	}

	// 子路由
	if action != "" {
		switch action {
		case "kill":
			killSession(id)
			writeJSON(w, 200, map[string]any{"ok": true, "killed": true})
			return
		case "detach":
			detachSession(id)
			writeJSON(w, 200, s.info())
			return
		case "attach":
			s.reattach()
			writeJSON(w, 200, s.info())
			return
		default:
			writeErr(w, 404, "unknown action: "+action)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, s.info())
	case http.MethodPatch, http.MethodPut, http.MethodPost:
		// 重命名：PATCH /api/sessions/{id} body: {"title": "new name"}
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, "invalid body")
			return
		}
		if body.Title == "" {
			writeErr(w, 400, "title is empty")
			return
		}
		// 截断超长 title (64 字符)
		if len(body.Title) > 64 {
			body.Title = body.Title[:64]
		}
		s.mu.Lock()
		s.Title = body.Title
		s.mu.Unlock()
		writeJSON(w, 200, s.info())
	case http.MethodDelete:
		// DELETE 改语义：detach（不杀进程，进 sidebar）
		// 真杀用 POST /api/sessions/{id}/kill
		detachSession(id)
		writeJSON(w, 200, s.info())
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settingsMu.RLock()
		s := settings
		settingsMu.RUnlock()
		writeJSON(w, 200, s)
	case http.MethodPut, http.MethodPost:
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, 400, "invalid body")
			return
		}
		settingsMu.Lock()
		if in.DefaultShell != "" {
			settings.DefaultShell = in.DefaultShell
		}
		if in.DefaultUser != "" {
			if _, err := lookupUser(in.DefaultUser); err == nil {
				settings.DefaultUser = in.DefaultUser
			}
		}
		if in.MaxSessions > 0 {
			settings.MaxSessions = in.MaxSessions
		}
		if in.HistoryBytes > 0 {
			settings.HistoryBytes = in.HistoryBytes
		}
		s := settings
		settingsMu.Unlock()
		if err := saveSettingsFile(); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, s)
	default:
		writeErr(w, 405, "method not allowed")
	}
}

// handleScripts: GET 列表 / POST 新建
func handleScripts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, listScripts())
	case http.MethodPost:
		var in struct {
			Name        string `json:"name"`
			Command     string `json:"command"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, 400, "invalid body")
			return
		}
		s, err := createScript(in.Name, in.Command, in.Description)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 201, *s)
	default:
		writeErr(w, 405, "method not allowed")
	}
}

// handleScriptByID: PATCH 修改 / DELETE 删除 / POST /{action} 特殊动作
func handleScriptByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/scripts/")
	if rest == "" {
		writeErr(w, 400, "missing id")
		return
	}
	var id, action string
	if idx := strings.Index(rest, "/"); idx >= 0 {
		id = rest[:idx]
		action = rest[idx+1:]
	} else {
		id = rest
	}

	if action != "" {
		switch action {
		case "run":
			if r.Method != http.MethodPost {
				writeErr(w, 405, "method not allowed")
				return
			}
			newID, _, err := runScript(id)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"session_id": newID})
		case "run-in-session":
			if r.Method != http.MethodPost {
				writeErr(w, 405, "method not allowed")
				return
			}
			var body struct {
				SessionID string `json:"session_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeErr(w, 400, "invalid body")
				return
			}
			if body.SessionID == "" {
				writeErr(w, 400, "session_id is empty")
				return
			}
			_, err := runScriptInSession(id, body.SessionID)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true})
		default:
			writeErr(w, 404, "unknown action: "+action)
		}
		return
	}

	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var in struct {
			Name        *string `json:"name"`
			Command     *string `json:"command"`
			Description *string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, 400, "invalid body")
			return
		}
		s, err := updateScript(id, in.Name, in.Command, in.Description)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, *s)
	case http.MethodDelete:
		if !deleteScript(id) {
			writeErr(w, 404, "script not found")
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		writeErr(w, 405, "method not allowed")
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		writeErr(w, 400, "missing session id")
		return
	}
	s := getSession(sessionID)
	if s == nil {
		writeErr(w, 404, "session not found")
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		appLogf("ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	cols, rows := readIntParam(r, "cols", s.Cols), readIntParam(r, "rows", s.Rows)
	// 只在尺寸真变化时才 resize (避免每次 attach 都触发 SIGWINCH 导致 zsh 画 PROMPT_EOL_MARK %)
	if cols > 0 && rows > 0 && (cols != s.Cols || rows != s.Rows) {
		s.resize(cols, rows)
	}

	sub := s.attach()

	conn.SetReadLimit(64 * 1024)

	if snap := s.buffer.Snapshot(); len(snap) > 0 {
		if err := conn.WriteMessage(websocket.BinaryMessage, snap); err != nil {
			return
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for data := range sub.ch {
			if len(data) > 8 && string(data[:8]) == "__EXIT__" {
				conn.WriteJSON(WSMessage{Type: WSMsgExit})
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	}()

	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	// 定期发送 PING 保持 WebSocket 存活，防止 fnOS 反向代理（nginx）因 proxy_read_timeout 切断空闲连接
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, []byte("keepalive"), time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		switch msgType {
		case websocket.TextMessage:
			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err == nil {
				switch msg.Type {
				case WSMsgResize:
					// 只在尺寸真变化时才 resize (避免频繁触发 SIGWINCH)
					if msg.Cols > 0 && msg.Rows > 0 && (msg.Cols != s.Cols || msg.Rows != s.Rows) {
						s.resize(msg.Cols, msg.Rows)
					}
				case WSMsgInput:
					if msg.Data != "" {
						s.write([]byte(msg.Data))
					}
				}
			}
		case websocket.BinaryMessage:
			if len(data) > 0 {
				s.write(data)
			}
		}
	}

	s.detachSub(sub) // 关闭 sub.ch → write goroutine 退出 → done 关闭 → 防止 goroutine 泄漏
	<-done
}

func readIntParam(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
