package main

import "time"

type SessionInfo struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	CreatedAt  string `json:"created_at"`
	DetachedAt string `json:"detached_at,omitempty"`
	Active     bool   `json:"active"`
	Detached   bool   `json:"detached"`
	Exited     bool   `json:"exited"`
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	Subs       int    `json:"subscribers"`
	Shell      string `json:"shell"`
	User       string `json:"user"`
	UserShell  string `json:"user_shell"`
	Pid        int    `json:"pid,omitempty"`
}

type SessionCreate struct {
	Title string `json:"title"`
	Shell string `json:"shell"`
	User  string `json:"user,omitempty"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

type WSMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Code int    `json:"code,omitempty"`
}

const (
	WSMsgResize = "resize"
	WSMsgExit   = "exit"
	WSMsgInput  = "input"

	defaultCols = 80
	defaultRows = 24

	maxBufferBytes = 512 * 1024
)

var _ = time.Now
