package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// PersistedSession 持久化的 session metadata (不含 process 状态, 启动时重新 attach 验证)
type PersistedSession struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	User       string `json:"user"`
	Shell      string `json:"shell"`
	CreatedAt  string `json:"created_at"`
	DetachedAt string `json:"detached_at"`
}

var (
	persistedMu  sync.Mutex
	persisted    = make(map[string]*PersistedSession)
	persistedDir = func() string {
		return filepath.Join(appVar, "sessions.json")
	}
)

// loadPersistedSessions 启动时 load detached sessions metadata
// 注意: process 不在内存中 (server 重启会清), load 只恢复 metadata
// 实际 process 是否还活由 PID 检查决定
func loadPersistedSessions() {
	persistedMu.Lock()
	defer persistedMu.Unlock()
	persisted = make(map[string]*PersistedSession)
	data, err := os.ReadFile(persistedDir())
	if err != nil {
		return // 文件不存在是正常的 (首次启动)
	}
	var arr []*PersistedSession
	if err := json.Unmarshal(data, &arr); err != nil {
		appLogf("load persisted sessions: %v", err)
		return
	}
	for _, s := range arr {
		persisted[s.ID] = s
	}
	appLogf("loaded %d persisted session metadata", len(persisted))
}

// savePersistedSessions 退出时 save 当前 detached sessions
func savePersistedSessions() error {
	persistedMu.Lock()
	defer persistedMu.Unlock()
	arr := make([]*PersistedSession, 0, len(persisted))
	for _, s := range persisted {
		arr = append(arr, s)
	}
	data, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(persistedDir(), data, 0o644)
}

// addPersistedSession 在 session detach 时调用
func addPersistedSession(s *Session) {
	persistedMu.Lock()
	persisted[s.ID] = &PersistedSession{
		ID:         s.ID,
		Title:      s.Title,
		User:       s.User,
		Shell:      s.Shell,
		CreatedAt:  s.CreatedAt.Format("2006-01-02 15:04:05"),
		DetachedAt: s.DetachedAt.Format("2006-01-02 15:04:05"),
	}
	persistedMu.Unlock()
}

// removePersistedSession 在 session kill / 真删时调用
func removePersistedSession(id string) {
	persistedMu.Lock()
	delete(persisted, id)
	persistedMu.Unlock()
}

// getPersistedSessions 给 API 用 (返回 detached sessions 列表)
// 同时检查 process 是否真的在跑, 不在跑的清理
func getPersistedSessions() []PersistedSession {
	// 同步内存中的 detached sessions 到 persisted
	sessionsMu.RLock()
	for id, s := range sessions {
		s.mu.Lock()
		detached := s.detached
		s.mu.Unlock()
		if detached {
			persistedMu.Lock()
			if _, ok := persisted[id]; !ok {
				persisted[id] = &PersistedSession{
					ID:         s.ID,
					Title:      s.Title,
					User:       s.User,
					Shell:      s.Shell,
					CreatedAt:  s.CreatedAt.Format("2006-01-02 15:04:05"),
					DetachedAt: s.DetachedAt.Format("2006-01-02 15:04:05"),
				}
			}
			persistedMu.Unlock()
		}
	}
	sessionsMu.RUnlock()
	persistedMu.Lock()
	out := make([]PersistedSession, 0, len(persisted))
	for _, s := range persisted {
		out = append(out, *s)
	}
	persistedMu.Unlock()
	return out
}

// prunePersistedDeadProcesses 启动时调用, 清理 process 已死的 persisted sessions
// 实际清理 (process 不在 = 之前 SIGKILL/SIGTERM, 但 metadata 还在磁盘)
func prunePersistedDeadProcesses() {
	persistedMu.Lock()
	defer persistedMu.Unlock()
	dead := []string{}
	for id := range persisted {
		// 找 process 是否活
		sessionsMu.RLock()
		s, ok := sessions[id]
		sessionsMu.RUnlock()
		if !ok || s == nil {
			// 内存没有 — 之前 server 重启后, 但 process 不知道在不在
			// 保守做法: 保留 (下次启动 process 仍可能活)
			continue
		}
		s.mu.Lock()
		alive := !s.exited && s.ptmx != nil
		s.mu.Unlock()
		if !alive {
			dead = append(dead, id)
		}
	}
	for _, id := range dead {
		delete(persisted, id)
	}
	if len(dead) > 0 {
		appLogf("pruned %d dead persisted sessions", len(dead))
		_ = savePersistedSessions()
	}
}

// formatLogPrefix 辅助
func formatLogPrefix(s string) string {
	return fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), s)
}
