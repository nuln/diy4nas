package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

var (
	sessionsMu sync.RWMutex
	sessions   = make(map[string]*Session)
)

func addSession(s *Session) {
	sessionsMu.Lock()
	sessions[s.ID] = s
	sessionsMu.Unlock()
}

func getSession(id string) *Session {
	sessionsMu.RLock()
	s := sessions[id]
	sessionsMu.RUnlock()
	return s
}

// listSessions 返回 session 列表
// includeDetached=true 包含所有（含 detached）；false 只返 active
func listSessions(includeDetached bool) []SessionInfo {
	sessionsMu.RLock()
	out := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		s.mu.Lock()
		detached := s.detached
		exited := s.exited
		s.mu.Unlock()
		if !includeDetached && (detached || exited) {
			continue
		}
		out = append(out, s.info())
	}
	sessionsMu.RUnlock()
	return out
}

// removeSession 真删（杀进程 + 从 map 移除）
func removeSession(id string) {
	sessionsMu.Lock()
	s := sessions[id]
	delete(sessions, id)
	sessionsMu.Unlock()
	if s != nil {
		s.close()
	}
}

// detachSession 标记 detached（不杀进程，保留在 map，进程继续在后台跑）
func detachSession(id string) bool {
	s := getSession(id)
	if s == nil {
		return false
	}
	s.detach()
	return true
}

// killSession 真杀（close + remove）
func killSession(id string) bool {
	s := getSession(id)
	if s == nil {
		return false
	}
	removeSession(id)
	return true
}

func createSession(in SessionCreate, user string) (*Session, error) {
	return createSessionOpt(in, user, false)
}

// createSessionOpt 创建 session, skipStart=true 时不启动 PTY (调用方自己 startWithCmd)
func createSessionOpt(in SessionCreate, user string, skipStart bool) (*Session, error) {
	id := randID()
	title := in.Title
	if title == "" {
		title = "terminal"
	}
	s := newSession(id, title, in.Shell, user, in.Cols, in.Rows)
	if !skipStart {
		if err := s.start(); err != nil {
			return nil, err
		}
	}
	addSession(s)
	return s, nil
}

// startAutoCleanup 启动后台清理 goroutine：每 1 小时扫一次
// detached 超 maxDetachedHours (默认 24h) 自动 kill
func startAutoCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			cleanupDetached()
		}
	}()
	// 启动后 1 分钟先扫一次
	go func() {
		time.Sleep(1 * time.Minute)
		cleanupDetached()
	}()
}

func cleanupDetached() {
	maxHours := 24
	sessionsMu.RLock()
	var toKill []string
	now := time.Now()
	for id, s := range sessions {
		s.mu.Lock()
		detached := s.detached
		exited := s.exited
		detachedAt := s.DetachedAt
		s.mu.Unlock()
		if !detached || exited {
			continue
		}
		if now.Sub(detachedAt) > time.Duration(maxHours)*time.Hour {
			toKill = append(toKill, id)
		}
	}
	sessionsMu.RUnlock()
	for _, id := range toKill {
		appLogf("cleanup: killing detached session %s (older than %dh)", id, maxHours)
		killSession(id)
	}
}

func randID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
