package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Script struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Command     string `json:"command"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	RunCount    int    `json:"run_count"`
	LastRunAt   string `json:"last_run_at,omitempty"`
}

var (
	scriptsMu sync.RWMutex
	scripts   = make(map[string]*Script)
)

func scriptPath() string {
	return filepath.Join(appVar, "scripts.json")
}

func loadScripts() {
	scriptsMu.Lock()
	defer scriptsMu.Unlock()
	scripts = make(map[string]*Script)
	data, err := os.ReadFile(scriptPath())
	if err != nil {
		// 首次启动: 文件不存在, 写一个默认示例脚本 (但不是强制的)
		// 注意: 已经在 write lock 里, 不能调 saveScripts (会死锁) — 直接写文件
		seedDefaultScriptsLocked()
		return
	}
	var arr []*Script
	if err := json.Unmarshal(data, &arr); err != nil {
		appLogf("load scripts: %v", err)
		return
	}
	for _, s := range arr {
		scripts[s.ID] = s
	}
}

func seedDefaultScriptsLocked() {
	// 调用方必须已持 scriptsMu 写锁
	now := time.Now().Format("2006-01-02 15:04:05")
	defaults := []*Script{
		{ID: randID(), Name: "系统更新", Command: "apt update && apt upgrade -y", Description: "更新系统包（默认）", CreatedAt: now, UpdatedAt: now},
		{ID: randID(), Name: "磁盘占用", Command: "df -h | head -20", Description: "查看磁盘占用前 20 行", CreatedAt: now, UpdatedAt: now},
	}
	for _, s := range defaults {
		scripts[s.ID] = s
	}
	// 写文件: 仍然在 lock 内 — 但内部不持 lock 不会死锁
	writeScriptsFile()
}

func saveScripts() error {
	scriptsMu.RLock()
	defer scriptsMu.RUnlock()
	return writeScriptsFile()
}

// writeScriptsFile 实际写文件 (调用方必须已持 scriptsMu 锁)
func writeScriptsFile() error {
	arr := make([]*Script, 0, len(scripts))
	for _, s := range scripts {
		arr = append(arr, s)
	}
	// 按 UpdatedAt 倒序
	sort.Slice(arr, func(i, j int) bool {
		return arr[i].UpdatedAt > arr[j].UpdatedAt
	})
	data, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(scriptPath(), data, 0o644)
}

func listScripts() []Script {
	scriptsMu.RLock()
	out := make([]Script, 0, len(scripts))
	for _, s := range scripts {
		out = append(out, *s)
	}
	scriptsMu.RUnlock()
	// 按 UpdatedAt 倒序
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out
}

func getScript(id string) *Script {
	scriptsMu.RLock()
	defer scriptsMu.RUnlock()
	if s, ok := scripts[id]; ok {
		// 返回副本
		copy := *s
		return &copy
	}
	return nil
}

func createScript(name, command, description string) (*Script, error) {
	name = strings.TrimSpace(name)
	command = strings.TrimSpace(command)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, errors.New("name is empty")
	}
	if command == "" {
		return nil, errors.New("command is empty")
	}
	if len(name) > 64 {
		name = name[:64]
	}
	if len(command) > 8192 {
		return nil, errors.New("command too long (max 8192)")
	}
	if len(description) > 256 {
		description = description[:256]
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	s := &Script{
		ID:          randID(),
		Name:        name,
		Command:     command,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	scriptsMu.Lock()
	scripts[s.ID] = s
	scriptsMu.Unlock()
	if err := saveScripts(); err != nil {
		return nil, err
	}
	return s, nil
}

func updateScript(id string, name, command, description *string) (*Script, error) {
	scriptsMu.Lock()
	s, ok := scripts[id]
	if !ok {
		scriptsMu.Unlock()
		return nil, errors.New("script not found")
	}
	if name != nil {
		v := strings.TrimSpace(*name)
		if v == "" {
			scriptsMu.Unlock()
			return nil, errors.New("name is empty")
		}
		if len(v) > 64 {
			v = v[:64]
		}
		s.Name = v
	}
	if command != nil {
		v := strings.TrimSpace(*command)
		if v == "" {
			scriptsMu.Unlock()
			return nil, errors.New("command is empty")
		}
		if len(v) > 8192 {
			scriptsMu.Unlock()
			return nil, errors.New("command too long (max 8192)")
		}
		s.Command = v
	}
	if description != nil {
		v := strings.TrimSpace(*description)
		if len(v) > 256 {
			v = v[:256]
		}
		s.Description = v
	}
	s.UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
	copy := *s
	scriptsMu.Unlock()
	if err := saveScripts(); err != nil {
		return nil, err
	}
	return &copy, nil
}

func deleteScript(id string) bool {
	scriptsMu.Lock()
	_, ok := scripts[id]
	if !ok {
		scriptsMu.Unlock()
		return false
	}
	delete(scripts, id)
	scriptsMu.Unlock()
	_ = saveScripts()
	return true
}

func markScriptRun(id string) {
	scriptsMu.Lock()
	s, ok := scripts[id]
	if !ok {
		scriptsMu.Unlock()
		return
	}
	s.RunCount++
	s.LastRunAt = time.Now().Format("2006-01-02 15:04:05")
	s.UpdatedAt = s.LastRunAt
	scriptsMu.Unlock()
	_ = saveScripts()
}

// runScript 创建一个临时 session 跑这个 script (新开 session 模式)
// 返回新 session ID
func runScript(id string) (string, *Script, error) {
	s := getScript(id)
	if s == nil {
		return "", nil, errors.New("script not found")
	}
	// 用 user + title = script name 创建 session
	settingsMu.RLock()
	maxN := settings.MaxSessions
	settingsMu.RUnlock()
	sessionsMu.RLock()
	count := len(sessions)
	sessionsMu.RUnlock()
	if count >= maxN {
		return "", nil, errors.New("session limit reached")
	}
	title := s.Name
	if len(title) > 64 {
		title = title[:64]
	}
	// resolve user
	user, _ := resolveRequestUser(nil)
	// 创建 session, skipStart=true 避免先启 login shell
	cols, rows := 80, 24
	newSess, err := createSessionOpt(SessionCreate{
		Title: title,
		Shell: "", // 用 -c, 不走 login
		Cols:  cols,
		Rows:  rows,
	}, user, true)
	if err != nil {
		return "", nil, err
	}
	// 直接用 -c 跑 script, 不走 login shell
	if err := newSess.startWithCmd(s.Command); err != nil {
		removeSession(newSess.ID)
		return "", nil, fmt.Errorf("start script session: %w", err)
	}
	markScriptRun(id)
	return newSess.ID, s, nil
}

// runScriptInSession 把 script 内容写到已有 session 的 PTY (在当前 active session 跑)
func runScriptInSession(scriptID, sessionID string) (*Script, error) {
	s := getScript(scriptID)
	if s == nil {
		return nil, errors.New("script not found")
	}
	sess := getSession(sessionID)
	if sess == nil {
		return nil, errors.New("session not found")
	}
	if sess.exited {
		return nil, errors.New("session exited")
	}
	// 写 script 到 PTY (加 \n 让 shell 执行)
	if err := sess.write([]byte(s.Command + "\n")); err != nil {
		return nil, err
	}
	markScriptRun(scriptID)
	return s, nil
}
