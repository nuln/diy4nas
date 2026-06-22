package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
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

	"github.com/gorilla/websocket"
)

//go:embed all:ui
var frontend embed.FS

var (
	appName   = "terminal"
	appDest   = envOr("TRIM_APPDEST", "/var/apps/terminal")
	appVar    = envOr("TRIM_PKGVAR", "/var/apps/terminal/data")
	servicePort = envOr("TRIM_SERVICE_PORT", "7682")
	logPath   = filepath.Join(appVar, "terminal.log")
	settingPath = filepath.Join(appVar, "terminal.settings.json")

	appLogFile *os.File
	settingsMu sync.RWMutex
	settings   Settings

	upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

type Settings struct {
	DefaultShell string `json:"default_shell"`
	DefaultUser  string `json:"default_user"`
	MaxSessions  int    `json:"max_sessions"`
	HistoryBytes int    `json:"history_bytes"`
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func appLogf(format string, args ...any) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := "[" + ts + "] " + fmt.Sprintf(format, args...)
	log.Print(line)
	if appLogFile != nil {
		appLogFile.WriteString(line + "\n")
	}
}

func loadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = Settings{DefaultShell: "/bin/bash", DefaultUser: detectDefaultUser(), MaxSessions: 8, HistoryBytes: maxBufferBytes}
	data, err := os.ReadFile(settingPath)
	if err != nil {
		return
	}
	var s Settings
	if json.Unmarshal(data, &s) == nil {
		if s.DefaultShell != "" {
			settings.DefaultShell = s.DefaultShell
		}
		if s.DefaultUser != "" {
			settings.DefaultUser = s.DefaultUser
		}
		if s.MaxSessions > 0 {
			settings.MaxSessions = s.MaxSessions
		}
		if s.HistoryBytes > 0 {
			settings.HistoryBytes = s.HistoryBytes
		}
	}
}

func saveSettingsFile() error {
	settingsMu.RLock()
	data, _ := json.MarshalIndent(settings, "", "  ")
	settingsMu.RUnlock()
	return os.WriteFile(settingPath, data, 0o644)
}

func detectDefaultUser() string {
	// 优先级：
	//   1. USER 环境变量（fnOS 框架 setuid 切到当前登录用户后跑 cmd/main，$USER 自动设）
	//   2. FNOS_USER 环境变量（cmd/main 显式透传）
	//   3. SUDO_USER 环境变量
	//   4. /etc/passwd 里的 admin 用户
	//   5. 第一个 UID >= 1000 且 shell 合法且非系统服务用户
	//   6. 留空
	if u := os.Getenv("USER"); u != "" && u != "root" {
		if isLoginUser(u) {
			return u
		}
	}
	if u := os.Getenv("FNOS_USER"); u != "" {
		if isLoginUser(u) {
			return u
		}
	}
	if u := os.Getenv("SUDO_USER"); u != "" && u != "root" {
		if isLoginUser(u) {
			return u
		}
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return ""
	}
	// 优先 admin（fnOS NAS 系统的标准登录用户）
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 7)
		if len(fields) < 7 {
			continue
		}
		if fields[0] == "admin" {
			// 验证 shell 合法
			sh := fields[6]
			if sh != "" && sh != "/usr/sbin/nologin" && sh != "/bin/false" {
				return "admin"
			}
		}
	}
	// 否则取 UID 最大的非 nobody 普通用户
	best := ""
	bestUID := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.SplitN(line, ":", 7)
		if len(fields) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(fields[2])
		name := fields[0]
		sh := fields[6]
		if uid < 1000 || uid >= 65534 || name == "nobody" {
			continue
		}
		if sh == "" || sh == "/usr/sbin/nologin" || sh == "/bin/false" {
			continue
		}
		if uid > bestUID {
			best = name
			bestUID = uid
		}
	}
	return best
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
			_ = os.WriteFile("/tmp/terminal-port", []byte(port), 0o644)
			break
		}
		p, _ := strconv.Atoi(port)
		port = strconv.Itoa(p + 1)
	}
	if ln == nil {
		log.Fatalf("no available port for terminal")
	}

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// 启动后台 detached session 自动清理（默认 24h）
	startAutoCleanup()

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
	ln.Close()
	sessionsMu.Lock()
	for _, s := range sessions {
		s.close()
	}
	sessions = make(map[string]*Session)
	sessionsMu.Unlock()
}

func openLogFile() error {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	appLogFile = f
	log.SetOutput(io.MultiWriter(os.Stdout, f))
	return nil
}
