package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
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
// includeDetached=true 包含所有（含 detached + persisted 历史的）
// false 只返 active
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
	if !includeDetached {
		return out
	}
	// includeDetached=true: 还包括 persisted 但不在内存 (server 重启后)
	persisted := getPersistedSessions()
	for _, p := range persisted {
		sessionsMu.RLock()
		_, inMem := sessions[p.ID]
		sessionsMu.RUnlock()
		if inMem {
			continue // 内存已有, 上面已加
		}
		// 转 SessionInfo (frontend 兼容)
		out = append(out, SessionInfo{
			ID:         p.ID,
			Title:      p.Title,
			CreatedAt:  p.CreatedAt,
			DetachedAt: p.DetachedAt,
			Detached:   true,
			Exited:     true, // process 不知道在不在, 标记 exited
			Cols:       80,
			Rows:       24,
			Shell:      p.Shell,
			User:       p.User,
		})
	}
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

// startAutoShutdown 启动自动关闭 goroutine: 每 5s 检查一次
// 条件: 所有 session 都 exited + 没有 detached session + 没有 ws client 连接
// -> server exit(0), fnOS 框架会关 app 窗口
func startAutoShutdown() {
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		// 启动后 60s 才开始检查 (避免刚启动时误判)
		time.Sleep(60 * time.Second)
		for range ticker.C {
			if canShutdown() {
				appLogf("auto-shutdown: all sessions exited, no detached sessions, no ws clients, exiting")
				os.Exit(0)
			}
		}
	}()
}

// canShutdown 检查是否可以安全退出
func canShutdown() bool {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	for _, s := range sessions {
		s.mu.Lock()
		exited := s.exited
		detached := s.detached
		subs := len(s.subs)
		s.mu.Unlock()
		// 任何 active (非 exited) 或 detached (后台运行) 的 session 都不退出
		if !exited || detached {
			return false
		}
		// 有 ws client 也不退出
		if subs > 0 {
			return false
		}
	}
	return true
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
		if exited {
			// exited + detached 也杀 (zombie), exited + !detached 正常走 60s cleanup
			if !detached {
				continue
			}
			// exited + detached: 清理僵尸
		} else if !detached {
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
