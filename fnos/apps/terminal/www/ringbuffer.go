package main

import "sync"

type RingBuffer struct {
	mu   sync.Mutex
	data []byte
	cap  int
}

func NewRingBuffer(capBytes int) *RingBuffer {
	if capBytes <= 0 {
		capBytes = 64 * 1024
	}
	return &RingBuffer{cap: capBytes}
}

func (r *RingBuffer) Write(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, p...)
	if len(r.data) > r.cap {
		cut := len(r.data) - r.cap
		if cut > 4096 {
			r.data = append(r.data[:0], r.data[cut:]...)
		} else {
			r.data = r.data[cut:]
		}
	}
}

func (r *RingBuffer) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, len(r.data))
	copy(out, r.data)
	return out
}

func (r *RingBuffer) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.data)
}
