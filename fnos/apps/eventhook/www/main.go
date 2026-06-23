package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed ui/index.html
var frontend embed.FS

var (
	appName    = "eventhook"
	appDest    = envOr("TRIM_APPDEST", "/var/apps/eventhook")
	appVar     = envOr("TRIM_PKGVAR", "/var/apps/eventhook/data")
	servicePort = envOr("TRIM_SERVICE_PORT", "7683")
	dbPath     = filepath.Join(appVar, "eventhook.db")
	logPath    = filepath.Join(appVar, "eventhook.log")
	settingPath = filepath.Join(appVar, "eventhook.settings.json")

	appLogFile *os.File
	db         *sql.DB

	settingsMu sync.RWMutex
	settings   Settings

	watcher *Watcher
)

func timeNow() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func main() {
	if err := os.MkdirAll(appVar, 0o755); err != nil {
		log.Fatalf("mkdir app var: %v", err)
	}
	if err := openLogFile(); err != nil {
		log.Fatalf("open log: %v", err)
	}
	defer appLogFile.Close()

	appLogf("starting eventhook server...")

	loadSettings()

	d, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	db = d
	defer db.Close()

	loadHooks()
	watcher = newWatcher()
	watcher.Start()

	mux := http.NewServeMux()
	registerRoutes(mux)

	port := servicePort
	var ln net.Listener
	for i := 0; i < 100; i++ {
		var err error
		ln, err = net.Listen("tcp", ":"+port)
		if err == nil {
			if port != servicePort {
				appLogf("port %s busy, using %s", servicePort, port)
			}
			_ = os.WriteFile("/tmp/eventhook-port", []byte(port), 0o644)
			break
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	if ln == nil {
		log.Fatalf("no available port for eventhook")
	}

	srv := &http.Server{
		Handler:      withCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		appLogf("listening on :%s", port)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			appLogf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	appLogf("shutting down...")

	watcher.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	ln.Close()
}

func openDB(path string) (*sql.DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := d.Ping(); err != nil {
		return nil, err
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS hooks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'webhook',
			enabled INTEGER NOT NULL DEFAULT 1,
			url TEXT DEFAULT '',
			token TEXT DEFAULT '',
			cmd TEXT DEFAULT '',
			headers TEXT DEFAULT '',
			event_types TEXT DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS event_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			detail TEXT DEFAULT '',
			result TEXT NOT NULL DEFAULT 'pending',
			hook_name TEXT DEFAULT '',
			error TEXT DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS cursor_pos (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id INTEGER NOT NULL
		)`,
	}
	for _, m := range migrations {
		if _, err := d.Exec(m); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func openLogFile() error {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	appLogFile = f
	log.SetOutput(f)
	return nil
}

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = Settings{
		PollInterval:  5,
		DedupWindow:   300,
		DndStart:      "",
		DndEnd:        "",
		EventloggerDB: "/usr/trim/var/eventlogger_service/logger_data.db3",
	}
	data, err := os.ReadFile(settingPath)
	if err != nil {
		return
	}
	var s Settings
	if json.Unmarshal(data, &s) == nil {
		if s.PollInterval > 0 {
			settings.PollInterval = s.PollInterval
		}
		if s.DedupWindow >= 0 {
			settings.DedupWindow = s.DedupWindow
		}
		if s.DndStart != "" {
			settings.DndStart = s.DndStart
		}
		if s.DndEnd != "" {
			settings.DndEnd = s.DndEnd
		}
		if s.EventloggerDB != "" {
			settings.EventloggerDB = s.EventloggerDB
		}
	}
}

func saveSettingsFile() error {
	settingsMu.RLock()
	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsMu.RUnlock()
	return os.WriteFile(settingPath, data, 0o644)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func appLogf(format string, args ...any) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] "+format, append([]any{ts}, args...)...)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if decoded, err := url.PathUnescape(p); err == nil {
			p = decoded
		}
		for strings.Contains(p, "//") {
			p = strings.ReplaceAll(p, "//", "/")
		}
		r.URL.Path = p
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		h.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	jsonResponse(w, code, APIError{Error: msg})
}
