package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type RunStream struct {
	mu       sync.Mutex
	stdout   *RingBuffer
	stderr   *RingBuffer
	closed   bool
	subs     map[chan []byte]struct{}
}

func newRunStream(maxBytes int) *RunStream {
	return &RunStream{
		stdout: NewRingBuffer(maxBytes),
		stderr: NewRingBuffer(maxBytes),
		subs:   make(map[chan []byte]struct{}),
	}
}

func (s *RunStream) writeLine(stream string, line string) {
	var rb *RingBuffer
	if stream == "stderr" {
		rb = s.stderr
	} else {
		rb = s.stdout
	}
	data := []byte(line + "\n")
	rb.Write(data)

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	payload := append([]byte{}, stream...)
	payload = append(payload, ':')
	payload = append(payload, data...)
	for ch := range s.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (s *RunStream) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for ch := range s.subs {
		close(ch)
		delete(s.subs, ch)
	}
}

func (s *RunStream) subscribe() (chan []byte, []byte, []byte) {
	ch := make(chan []byte, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	stdoutSnap := s.stdout.Snapshot()
	stderrSnap := s.stderr.Snapshot()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		close(ch)
	}
	return ch, stdoutSnap, stderrSnap
}

func (s *RunStream) unsubscribe(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
}

func executeJob(jobID int64, trigger string) {
	job, err := db.GetJob(jobID)
	if err != nil {
		appLogf("get job %d: %v", jobID, err)
		return
	}
	if job == nil {
		appLogf("job %d not found", jobID)
		return
	}
	if !job.Enabled {
		return
	}

	settingsMu.RLock()
	defaultTimeout := settings.DefaultTimeout
	settingsMu.RUnlock()

	timeoutSec := job.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = defaultTimeout
	}
	if timeoutSec <= 0 {
		timeoutSec = 3600
	}

	runID, err := db.CreateRun(jobID, trigger)
	if err != nil {
		appLogf("create run for job %d: %v", jobID, err)
		return
	}
	appLogf("job %d (%s) start run %d trigger=%s", jobID, job.Name, runID, trigger)

	stream := newRunStream(maxLogFileBytes)
	runMu.Lock()
	runStreams[runID] = stream
	runMu.Unlock()
	defer func() {
		runMu.Lock()
		delete(runStreams, runID)
		runMu.Unlock()
		stream.close()
	}()

	started := time.Now()
	runStartTimes[runID] = started
	defer delete(runStartTimes, runID)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// 解析执行用户 (per-task user, fallback 到 FNOS_USER -> root)
	uid, gid, userHome, runAsUser, userErr := resolveJobUser(job.Username)
	if userErr != nil {
		appLogf("job %d (%s): %v - falling back to root", jobID, job.Name, userErr)
		uid, gid, userHome, runAsUser = 0, 0, "/root", "root"
	} else if job.Username != "" {
		appLogf("job %d (%s) will run as user: %s (uid=%d gid=%d home=%s)", jobID, job.Name, runAsUser, uid, gid, userHome)
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", job.Command)
	cmd.Env = append(os.Environ(),
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME="+userHome,
		"USER="+runAsUser,
		"LOGNAME="+runAsUser,
		"LANG=zh_CN.UTF-8",
		"LC_ALL=zh_CN.UTF-8",
	)
	// setuid 必须在 Start() 之前设置 (SysProcAttr 由 fork exec 时使用)
	cmd.SysProcAttr = credentialFor(uid, gid)
	// 工作目录: job.Workdir 优先, 否则用 data dir (确保存在)
	// 不直接用 userHome 因为 nobody 等系统用户的 home 可能不存在
	if job.Workdir != "" {
		cmd.Dir = job.Workdir
	} else {
		cmd.Dir = appVar
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		db.FinishRun(runID, StatusFailed, -1, time.Since(started).Milliseconds(), "", "stdout pipe: "+err.Error())
		appLogf("job %d run %d stdout pipe: %v", jobID, runID, err)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		db.FinishRun(runID, StatusFailed, -1, time.Since(started).Milliseconds(), "", "stderr pipe: "+err.Error())
		appLogf("job %d run %d stderr pipe: %v", jobID, runID, err)
		return
	}

	if err := cmd.Start(); err != nil {
		db.FinishRun(runID, StatusFailed, -1, time.Since(started).Milliseconds(), "", "start: "+err.Error())
		appLogf("job %d run %d start: %v", jobID, runID, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanPipe(stdoutPipe, "stdout", stream)
	}()
	go func() {
		defer wg.Done()
		scanPipe(stderrPipe, "stderr", stream)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	duration := time.Since(started)

	status := StatusSuccess
	exitCode := 0
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			status = StatusTimeout
		} else {
			status = StatusFailed
		}
		if ee, ok := waitErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	stdoutSnap := stream.stdout.SnapshotString()
	stderrSnap := stream.stderr.SnapshotString()
	if err := db.FinishRun(runID, status, exitCode, duration.Milliseconds(), stdoutSnap, stderrSnap); err != nil {
		appLogf("finish run %d: %v", runID, err)
	}
	if err := db.UpdateJobLastRun(jobID, status); err != nil {
		appLogf("update job %d last_run: %v", jobID, err)
	}
	appLogf("job %d run %d done status=%s exit=%d duration=%s", jobID, runID, status, exitCode, duration)
}

func scanPipe(r io.Reader, name string, stream *RunStream) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		stream.writeLine(name, scanner.Text())
	}
	if err := scanner.Err(); err != nil && !strings.Contains(err.Error(), "file already closed") {
		appLogf("scan %s: %v", name, err)
	}
}

func getStream(runID int64) *RunStream {
	runMu.Lock()
	defer runMu.Unlock()
	return runStreams[runID]
}

func executeJobSync(jobID int64, trigger string) error {
	executeJob(jobID, trigger)
	return nil
}

// 解析命令为参数数组（不实际使用，保留用于未来扩展）
func parseCommand(cmd string) []string {
	return strings.Fields(cmd)
}

// 测试用辅助
func describeRun(runID int64) string {
	r, err := db.GetRun(runID)
	if err != nil || r == nil {
		return fmt.Sprintf("run %d not found", runID)
	}
	return fmt.Sprintf("run %d status=%s exit=%d", r.ID, r.Status, r.ExitCode)
}
