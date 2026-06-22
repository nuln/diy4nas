package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
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

func listSessions() []SessionInfo {
	sessionsMu.RLock()
	out := make([]SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.info())
	}
	sessionsMu.RUnlock()
	return out
}

func removeSession(id string) {
	sessionsMu.Lock()
	s := sessions[id]
	delete(sessions, id)
	sessionsMu.Unlock()
	if s != nil {
		s.close()
	}
}

func createSession(in SessionCreate, user string) (*Session, error) {
	id := randID()
	title := in.Title
	if title == "" {
		title = "terminal"
	}
	s := newSession(id, title, in.Shell, user, in.Cols, in.Rows)
	if err := s.start(); err != nil {
		return nil, err
	}
	addSession(s)
	return s, nil
}

func randID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
