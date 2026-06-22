package main

import "time"

type Job struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Spec         string `json:"spec"`
	Command      string `json:"command"`
	Workdir      string `json:"workdir"`
	Enabled      bool   `json:"enabled"`
	Description  string `json:"description"`
	NotifyOn     string `json:"notify_on"`
	TimeoutSec   int    `json:"timeout_sec"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	NextRun      string `json:"next_run,omitempty"`
	LastStatus   string `json:"last_status,omitempty"`
	LastRunAt    string `json:"last_run_at,omitempty"`
}

type Run struct {
	ID         int64  `json:"id"`
	JobID      int64  `json:"job_id"`
	JobName    string `json:"job_name,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Trigger    string `json:"trigger"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
}

type Settings struct {
	Timezone       string `json:"timezone"`
	DefaultTimeout int    `json:"default_timeout_sec"`
	MaxLogBytes    int    `json:"max_log_bytes"`
}

type Stats struct {
	TotalJobs    int            `json:"total_jobs"`
	EnabledJobs  int            `json:"enabled_jobs"`
	TotalRuns    int            `json:"total_runs"`
	SuccessRuns  int            `json:"success_runs"`
	FailedRuns   int            `json:"failed_runs"`
	RecentRuns   []Run          `json:"recent_runs"`
	RecentJobs   []Job          `json:"recent_jobs"`
}

type APIError struct {
	Error string `json:"error"`
}

type JobInput struct {
	Name        string `json:"name"`
	Spec        string `json:"spec"`
	Command     string `json:"command"`
	Workdir     string `json:"workdir"`
	Enabled     *bool  `json:"enabled"`
	Description string `json:"description"`
	NotifyOn    string `json:"notify_on"`
	TimeoutSec  int    `json:"timeout_sec"`
}

const (
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"

	NotifyNone    = "none"
	NotifyAlways  = "always"
	NotifyFailure = "failure"

	TriggerScheduled = "scheduled"
	TriggerManual   = "manual"

	maxLogPreviewBytes = 64 * 1024
	maxLogFileBytes    = 2 * 1024 * 1024
)

var runStartTimes = make(map[int64]time.Time)
