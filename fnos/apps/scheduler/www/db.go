package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
	mu   sync.Mutex
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS jobs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    spec          TEXT NOT NULL,
    command       TEXT NOT NULL,
    workdir       TEXT DEFAULT '',
    username      TEXT DEFAULT '',
    enabled       INTEGER NOT NULL DEFAULT 1,
    description   TEXT DEFAULT '',
    notify_on     TEXT DEFAULT 'failure',
    timeout_sec   INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_status   TEXT DEFAULT '',
    last_run_at   TEXT DEFAULT ''
);
CREATE TABLE IF NOT EXISTS runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id        INTEGER NOT NULL,
    started_at    DATETIME NOT NULL,
    finished_at   DATETIME,
    status        TEXT NOT NULL DEFAULT 'running',
    exit_code     INTEGER NOT NULL DEFAULT 0,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    trigger       TEXT NOT NULL DEFAULT 'scheduled',
    stdout        TEXT DEFAULT '',
    stderr        TEXT DEFAULT '',
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_jobs_enabled ON jobs(enabled);
CREATE INDEX IF NOT EXISTS idx_runs_job_started ON runs(job_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status ON runs(status);
`

func migrateSchema(conn *sql.DB) error {
	for _, col := range []string{"last_status TEXT DEFAULT ''", "last_run_at TEXT DEFAULT ''", "username TEXT DEFAULT ''"} {
		var n int
		row := conn.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('jobs') WHERE name = ?`, strings.SplitN(col, " ", 2)[0])
		if err := row.Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if _, err := conn.Exec(`ALTER TABLE jobs ADD COLUMN ` + col); err != nil {
				return err
			}
		}
	}
	return nil
}

func openDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("schema: %w", err)
	}
	if err := migrateSchema(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{conn: conn}, nil
}

func (d *DB) Close() error { return d.conn.Close() }

func (d *DB) RecoverInterruptedRuns() error {
	_, err := d.conn.Exec(`UPDATE runs SET status = 'interrupted', finished_at = CURRENT_TIMESTAMP WHERE status = 'running'`)
	return err
}

func (d *DB) ListJobs() ([]Job, error) {
	rows, err := d.conn.Query(`SELECT id, name, spec, command, workdir, COALESCE(username, ''), enabled, description, notify_on, timeout_sec, created_at, updated_at, COALESCE(last_status, ''), COALESCE(last_run_at, '') FROM jobs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var j Job
		var enabled int
		if err := rows.Scan(&j.ID, &j.Name, &j.Spec, &j.Command, &j.Workdir, &j.Username, &enabled, &j.Description, &j.NotifyOn, &j.TimeoutSec, &j.CreatedAt, &j.UpdatedAt, &j.LastStatus, &j.LastRunAt); err != nil {
			return nil, err
		}
		j.Enabled = enabled == 1
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (d *DB) GetJob(id int64) (*Job, error) {
	row := d.conn.QueryRow(`SELECT id, name, spec, command, workdir, COALESCE(username, ''), enabled, description, notify_on, timeout_sec, created_at, updated_at, COALESCE(last_status, ''), COALESCE(last_run_at, '') FROM jobs WHERE id = ?`, id)
	var j Job
	var enabled int
	if err := row.Scan(&j.ID, &j.Name, &j.Spec, &j.Command, &j.Workdir, &j.Username, &enabled, &j.Description, &j.NotifyOn, &j.TimeoutSec, &j.CreatedAt, &j.UpdatedAt, &j.LastStatus, &j.LastRunAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	j.Enabled = enabled == 1
	return &j, nil
}

func (d *DB) CreateJob(in JobInput) (int64, error) {
	enabled := 1
	if in.Enabled != nil && !*in.Enabled {
		enabled = 0
	}
	res, err := d.conn.Exec(`INSERT INTO jobs (name, spec, command, workdir, username, enabled, description, notify_on, timeout_sec) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Name, in.Spec, in.Command, in.Workdir, in.Username, enabled, in.Description, in.NotifyOn, in.TimeoutSec)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) UpdateJob(id int64, in JobInput) error {
	enabled := 1
	if in.Enabled != nil && !*in.Enabled {
		enabled = 0
	}
	_, err := d.conn.Exec(`UPDATE jobs SET name = ?, spec = ?, command = ?, workdir = ?, username = ?, enabled = ?, description = ?, notify_on = ?, timeout_sec = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		in.Name, in.Spec, in.Command, in.Workdir, in.Username, enabled, in.Description, in.NotifyOn, in.TimeoutSec, id)
	return err
}

func (d *DB) DeleteJob(id int64) error {
	_, err := d.conn.Exec(`DELETE FROM jobs WHERE id = ?`, id)
	return err
}

func (d *DB) ToggleJob(id int64) (bool, error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var enabled int
	if err := tx.QueryRow(`SELECT enabled FROM jobs WHERE id = ?`, id).Scan(&enabled); err != nil {
		return false, err
	}
	enabled = 1 - enabled
	if _, err := tx.Exec(`UPDATE jobs SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, enabled, id); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return enabled == 1, nil
}

func (d *DB) CreateRun(jobID int64, trigger string) (int64, error) {
	res, err := d.conn.Exec(`INSERT INTO runs (job_id, started_at, status, trigger) VALUES (?, ?, 'running', ?)`, jobID, timeNow().Format("2006-01-02 15:04:05"), trigger)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) FinishRun(id int64, status string, exitCode int, durationMs int64, stdout, stderr string) error {
	_, err := d.conn.Exec(`UPDATE runs SET finished_at = ?, status = ?, exit_code = ?, duration_ms = ?, stdout = ?, stderr = ? WHERE id = ?`,
		timeNow().Format("2006-01-02 15:04:05"), status, exitCode, durationMs, truncate(stdout, maxLogPreviewBytes), truncate(stderr, maxLogPreviewBytes), id)
	return err
}

func (d *DB) UpdateJobLastRun(id int64, status string) error {
	_, err := d.conn.Exec(`UPDATE jobs SET last_status = ?, last_run_at = ? WHERE id = ?`,
		status, timeNow().Format("2006-01-02 15:04:05"), id)
	return err
}

func (d *DB) GetRun(id int64) (*Run, error) {
	row := d.conn.QueryRow(`SELECT r.id, r.job_id, COALESCE(j.name, ''), r.started_at, COALESCE(r.finished_at, ''), r.status, r.exit_code, r.duration_ms, r.trigger, COALESCE(r.stdout, ''), COALESCE(r.stderr, '') FROM runs r LEFT JOIN jobs j ON r.job_id = j.id WHERE r.id = ?`, id)
	var r Run
	if err := row.Scan(&r.ID, &r.JobID, &r.JobName, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ExitCode, &r.DurationMs, &r.Trigger, &r.Stdout, &r.Stderr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (d *DB) ListRuns(jobID int64, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT r.id, r.job_id, COALESCE(j.name, ''), r.started_at, COALESCE(r.finished_at, ''), r.status, r.exit_code, r.duration_ms, r.trigger FROM runs r LEFT JOIN jobs j ON r.job_id = j.id`
	args := []any{}
	if jobID > 0 {
		q += ` WHERE r.job_id = ?`
		args = append(args, jobID)
	}
	q += ` ORDER BY r.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := d.conn.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.JobID, &r.JobName, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ExitCode, &r.DurationMs, &r.Trigger); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (d *DB) CountRuns(status string) (int, error) {
	var n int
	q := `SELECT COUNT(*) FROM runs`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	if err := d.conn.QueryRow(q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (d *DB) DeleteOldRuns(keepDays int) (int64, error) {
	if keepDays <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).UTC().Format("2006-01-02 15:04:05")
	res, err := d.conn.Exec(`DELETE FROM runs WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-max:]
}

func joinArgs(args []string) string { return strings.Join(args, " ") }
