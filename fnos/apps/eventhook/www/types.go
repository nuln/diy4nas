package main

type Hook struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Enabled    bool     `json:"enabled"`
	URL        string   `json:"url,omitempty"`
	Token      string   `json:"token,omitempty"`
	Cmd        string   `json:"cmd,omitempty"`
	Headers    string   `json:"headers,omitempty"`
	EventTypes []string `json:"event_types"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

type HookInput struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Enabled    *bool    `json:"enabled"`
	URL        string   `json:"url"`
	Token      string   `json:"token"`
	Cmd        string   `json:"cmd"`
	Headers    string   `json:"headers"`
	EventTypes []string `json:"event_types"`
}

type EventRecord struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Detail    string `json:"detail"`
	Result    string `json:"result"`
	HookName  string `json:"hook_name,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Settings struct {
	PollInterval  int    `json:"poll_interval"`
	DedupWindow   int    `json:"dedup_window"`
	DndStart      string `json:"dnd_start"`
	DndEnd        string `json:"dnd_end"`
	EventloggerDB string `json:"eventlogger_db"`
}

type Stats struct {
	TotalEvents   int            `json:"total_events"`
	SentEvents    int            `json:"sent_events"`
	SkippedEvents int            `json:"skipped_events"`
	TotalHooks    int            `json:"total_hooks"`
	WatcherRunning bool          `json:"watcher_running"`
	RecentEvents  []EventRecord  `json:"recent_events"`
}

type APIError struct {
	Error string `json:"error"`
}

type HealthInfo struct {
	OK           bool     `json:"ok"`
	Time         string   `json:"time"`
	Watcher      bool     `json:"watcher"`
	WatcherError string   `json:"watcher_error"`
	Cursor       int64    `json:"cursor"`
	DBPath       string   `json:"db_path"`
	DBReachable  bool     `json:"db_reachable"`
	DBTables     []string `json:"db_tables"`
	DBError      string   `json:"db_error"`
	TotalEvents  int      `json:"total_events"`
	HooksCount   int      `json:"hooks_count"`
}

const (
	ResultPending = "pending"
	ResultSent    = "sent"
	ResultSkipped = "skipped"
	ResultFailed  = "failed"
)
