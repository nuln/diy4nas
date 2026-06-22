package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

func registerRoutes(mux *http.ServeMux) {
	api := http.NewServeMux()

	api.HandleFunc("/api/healthz", handleHealthz)
	api.HandleFunc("/api/jobs", handleJobs)
	api.HandleFunc("/api/jobs/", handleJobByID)
	api.HandleFunc("/api/runs", handleRuns)
	api.HandleFunc("/api/runs/", handleRunByID)
	api.HandleFunc("/api/stats", handleStats)
	api.HandleFunc("/api/settings", handleSettings)
	api.HandleFunc("/api/log", handleLog)
	api.HandleFunc("/api/cleanup", handleCleanup)

	mux.Handle("/api/", api)
	mux.HandleFunc("/", handleIndex)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/app/") {
		http.NotFound(w, r)
		return
	}
	data, err := frontend.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "frontend missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]any{"ok": true, "time": timeNow().Format("2006-01-02 15:04:05")})
}

func handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listJobs(w, r)
	case http.MethodPost:
		createJob(w, r)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleJobByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, 400, "missing id")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, 400, "invalid id")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		getJob(w, id)
	case action == "" && r.Method == http.MethodPut:
		updateJob(w, r, id)
	case action == "" && r.Method == http.MethodDelete:
		deleteJob(w, id)
	case action == "run" && r.Method == http.MethodPost:
		runJobNow(w, id)
	case action == "toggle" && r.Method == http.MethodPost:
		toggleJob(w, id)
	case action == "runs" && r.Method == http.MethodGet:
		listJobRuns(w, r, id)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	jobID := parseIntDefault(r.URL.Query().Get("job_id"), 0)
	runs, err := db.ListRuns(int64(jobID), limit)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if runs == nil {
		runs = []Run{}
	}
	jsonResponse(w, 200, runs)
}

func handleRunByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, 400, "missing id")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		jsonError(w, 400, "invalid id")
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		run, err := db.GetRun(id)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		if run == nil {
			jsonError(w, 404, "run not found")
			return
		}
		jsonResponse(w, 200, run)
	case action == "log" && r.Method == http.MethodGet:
		streamRunLog(w, r, id)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	jobs, err := db.ListJobs()
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	total := len(jobs)
	enabled := 0
	for _, j := range jobs {
		if j.Enabled {
			enabled++
		}
	}
	success, _ := db.CountRuns(StatusSuccess)
	failed, _ := db.CountRuns(StatusFailed)
	totalRuns := success + failed

	recentRuns, _ := db.ListRuns(0, 10)
	if recentRuns == nil {
		recentRuns = []Run{}
	}

	recentJobs := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		j.NextRun = cronNextRun(j.ID)
		recentJobs = append(recentJobs, j)
	}

	jsonResponse(w, 200, Stats{
		TotalJobs: total, EnabledJobs: enabled,
		TotalRuns: totalRuns, SuccessRuns: success, FailedRuns: failed,
		RecentRuns: recentRuns, RecentJobs: recentJobs,
	})
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settingsMu.RLock()
		s := settings
		settingsMu.RUnlock()
		jsonResponse(w, 200, s)
	case http.MethodPut, http.MethodPost:
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			jsonError(w, 400, "invalid body")
			return
		}
		settingsMu.Lock()
		tzChanged := false
		if in.Timezone != "" && in.Timezone != settings.Timezone {
			settings.Timezone = in.Timezone
			tzChanged = true
		}
		if in.DefaultTimeout > 0 {
			settings.DefaultTimeout = in.DefaultTimeout
		}
		if in.MaxLogBytes > 0 {
			settings.MaxLogBytes = in.MaxLogBytes
		}
		s := settings
		settingsMu.Unlock()
		if err := saveSettingsFile(); err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		// 时区变更需重建 cron（cron location 是初始化时绑定）
		if tzChanged {
			serverCmd.Stop()
			serverCmd = cron.New(cron.WithLocation(loadLocation()), cron.WithSeconds())
			if err := syncJobsToCron(); err != nil {
				appLogf("rebuild cron after tz change: %v", err)
			}
			serverCmd.Start()
		}
		jsonResponse(w, 200, s)
	default:
		jsonError(w, 405, "method not allowed")
	}
}

func listJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := db.ListJobs()
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	for i := range jobs {
		jobs[i].NextRun = cronNextRun(jobs[i].ID)
	}
	if jobs == nil {
		jobs = []Job{}
	}
	jsonResponse(w, 200, jobs)
}

func getJob(w http.ResponseWriter, id int64) {
	job, err := db.GetJob(id)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if job == nil {
		jsonError(w, 404, "job not found")
		return
	}
	job.NextRun = cronNextRun(id)
	jsonResponse(w, 200, job)
}

func createJob(w http.ResponseWriter, r *http.Request) {
	var in JobInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, 400, "invalid body")
		return
	}
	if err := validateJobInput(&in); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	id, err := db.CreateJob(in)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if err := syncJobsToCron(); err != nil {
		appLogf("sync after create: %v", err)
	}
	job, _ := db.GetJob(id)
	job.NextRun = cronNextRun(id)
	jsonResponse(w, 201, job)
}

func updateJob(w http.ResponseWriter, r *http.Request, id int64) {
	var in JobInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, 400, "invalid body")
		return
	}
	if err := validateJobInput(&in); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	job, _ := db.GetJob(id)
	if job == nil {
		jsonError(w, 404, "job not found")
		return
	}
	if err := db.UpdateJob(id, in); err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if err := syncJobsToCron(); err != nil {
		appLogf("sync after update: %v", err)
	}
	updated, _ := db.GetJob(id)
	updated.NextRun = cronNextRun(id)
	jsonResponse(w, 200, updated)
}

func deleteJob(w http.ResponseWriter, id int64) {
	job, _ := db.GetJob(id)
	if job == nil {
		jsonError(w, 404, "job not found")
		return
	}
	if err := db.DeleteJob(id); err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	removeCronEntry(id)
	jsonResponse(w, 200, map[string]any{"ok": true})
}

func runJobNow(w http.ResponseWriter, id int64) {
	job, _ := db.GetJob(id)
	if job == nil {
		jsonError(w, 404, "job not found")
		return
	}
	go executeJob(id, TriggerManual)
	jsonResponse(w, 202, map[string]any{"ok": true, "message": "任务已加入执行队列"})
}

func toggleJob(w http.ResponseWriter, id int64) {
	job, _ := db.GetJob(id)
	if job == nil {
		jsonError(w, 404, "job not found")
		return
	}
	enabled, err := db.ToggleJob(id)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	syncJobsToCron()
	jsonResponse(w, 200, map[string]any{"enabled": enabled})
}

func listJobRuns(w http.ResponseWriter, r *http.Request, id int64) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	runs, err := db.ListRuns(id, limit)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if runs == nil {
		runs = []Run{}
	}
	jsonResponse(w, 200, runs)
}

func streamRunLog(w http.ResponseWriter, r *http.Request, id int64) {
	run, err := db.GetRun(id)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	if run == nil {
		jsonError(w, 404, "run not found")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, 500, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	stream := getStream(id)
	ctx := r.Context()

	if run.Status != StatusRunning || stream == nil {
		sendSSE(w, "stdout", run.Stdout)
		sendSSE(w, "stderr", run.Stderr)
		sendSSE(w, "done", run.Status)
		flusher.Flush()
		return
	}

	ch, stdoutSnap, stderrSnap := stream.subscribe()
	defer stream.unsubscribe(ch)

	if len(stdoutSnap) > 0 {
		sendSSE(w, "stdout", string(stdoutSnap))
	}
	if len(stderrSnap) > 0 {
		sendSSE(w, "stderr", string(stderrSnap))
	}
	flusher.Flush()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case data, ok := <-ch:
			if !ok {
				latest, _ := db.GetRun(id)
				if latest != nil {
					sendSSE(w, "done", latest.Status)
					flusher.Flush()
				}
				return
			}
			sep := bytesIndexByte(data, ':')
			if sep < 0 {
				continue
			}
			sendSSE(w, string(data[:sep]), string(data[sep+1:]))
			flusher.Flush()
		}
	}
}

func sendSSE(w http.ResponseWriter, event, data string) {
	if data == "" {
		return
	}
	lines := strings.Split(data, "\n")
	for _, ln := range lines {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, ln)
	}
}

func bytesIndexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

func validateJobInput(in *JobInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("name 不能为空")
	}
	if strings.TrimSpace(in.Spec) == "" {
		return fmt.Errorf("spec 不能为空")
	}
	if strings.TrimSpace(in.Command) == "" {
		return fmt.Errorf("command 不能为空")
	}
	if in.NotifyOn == "" {
		in.NotifyOn = NotifyFailure
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(in.Spec); err != nil {
		return fmt.Errorf("cron 表达式无效: %v", err)
	}
	switch in.NotifyOn {
	case "", NotifyNone, NotifyAlways, NotifyFailure:
	default:
		return fmt.Errorf("notify_on 必须是 none/always/failure 之一")
	}
	if in.TimeoutSec < 0 {
		return fmt.Errorf("timeout_sec 不能为负数")
	}
	return nil
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return def
}

func handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	lines := parseIntDefault(r.URL.Query().Get("lines"), 200)
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	all := strings.Split(string(data), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(strings.Join(all, "\n")))
}

func handleCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, 405, "method not allowed")
		return
	}
	days := parseIntDefault(r.URL.Query().Get("days"), 30)
	if days < 1 {
		days = 30
	}
	deleted, err := db.DeleteOldRuns(days)
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	jsonResponse(w, 200, map[string]any{"deleted": deleted, "keep_days": days})
}

var _ = context.Background
