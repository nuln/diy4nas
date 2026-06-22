package main

import "time"

type SessionInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	Active    bool   `json:"active"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
	Subs      int    `json:"subscribers"`
	Shell     string `json:"shell"`
	User      string `json:"user"`
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
