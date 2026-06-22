package main

import (
	"context"
	"embed"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

//go:embed ui/index.html
var frontend embed.FS

var (
	appName   = "scheduler"
	appDest   = envOr("TRIM_APPDEST", "/var/apps/scheduler")
	appVar    = envOr("TRIM_PKGVAR", "/var/apps/scheduler/data")
	sockPath  = envOr("TRIM_SOCKET", filepath.Join(appVar, appName+".sock")) // 保留兼容性
	servicePort = envOr("TRIM_SERVICE_PORT", "7681")
	dbPath    = filepath.Join(appVar, "scheduler.db")
	logPath   = filepath.Join(appVar, "scheduler.log")
	settingPath = filepath.Join(appVar, "scheduler.settings.json")

	appLogFile *os.File
	serverCmd  *cron.Cron
	db         *DB

	settingsMu sync.RWMutex
	settings   Settings

	runMu      sync.Mutex
	runStreams = make(map[int64]*RunStream)
	jobMu      sync.Mutex
)

func timeNow() time.Time {
	settingsMu.RLock()
	loc := settings.Timezone
	settingsMu.RUnlock()
	if loc == "" {
		return time.Now()
	}
	if l, err := time.LoadLocation(loc); err == nil {
		return time.Now().In(l)
	}
	return time.Now()
}

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = Settings{Timezone: "Asia/Shanghai", DefaultTimeout: 3600, MaxLogBytes: maxLogFileBytes}
	data, err := os.ReadFile(settingPath)
	if err != nil {
		return
	}
	var s Settings
	if json.Unmarshal(data, &s) == nil {
		if s.Timezone != "" {
			settings.Timezone = s.Timezone
		}
		if s.DefaultTimeout > 0 {
			settings.DefaultTimeout = s.DefaultTimeout
		}
		if s.MaxLogBytes > 0 {
			settings.MaxLogBytes = s.MaxLogBytes
		}
	}
}

func saveSettingsFile() error {
	settingsMu.RLock()
	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsMu.RUnlock()
	return os.WriteFile(settingPath, data, 0o644)
}

func main() {
	if err := os.MkdirAll(appVar, 0o755); err != nil {
		log.Fatalf("mkdir app var: %v", err)
	}
	if err := openLogFile(); err != nil {
		log.Fatalf("open log: %v", err)
	}
	defer appLogFile.Close()

	loadSettings()

	d, err := openDB(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	db = d
	defer db.Close()

	if err := db.RecoverInterruptedRuns(); err != nil {
		appLogf("recover interrupted runs: %v", err)
	}

	serverCmd = cron.New(cron.WithLocation(loadLocation()), cron.WithSeconds())
	if err := syncJobsToCron(); err != nil {
		appLogf("sync jobs to cron: %v", err)
	}
	serverCmd.Start()

	mux := http.NewServeMux()
	registerRoutes(mux)

	port := servicePort
	var ln net.Listener
	for i := 0; i < 100; i++ {
		var err error
		ln, err = net.Listen("tcp", "127.0.0.1:"+port)
		if err == nil {
			if port != servicePort {
				appLogf("port %s busy, using %s", servicePort, port)
			}
			_ = os.WriteFile("/tmp/scheduler-port", []byte(port), 0o644)
			break
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	if ln == nil {
		log.Fatalf("no available port for scheduler")
	}

	srv := &http.Server{
		Handler:      withCORS(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		appLogf("listening on http://0.0.0.0:%s", port)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			appLogf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	appLogf("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	serverCmd.Stop()
	ln.Close()
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

func loadLocation() *time.Location {
	settingsMu.RLock()
	tz := settings.Timezone
	settingsMu.RUnlock()
	if tz == "" {
		return time.Local
	}
	if l, err := time.LoadLocation(tz); err == nil {
		return l
	}
	return time.Local
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
