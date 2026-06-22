package main

import (
	"fmt"

	"github.com/robfig/cron/v3"
)

type cronEntry struct {
	jobID int64
	id    cron.EntryID
}

var cronEntries = make(map[int64]cronEntry)

func syncJobsToCron() error {
	jobs, err := db.ListJobs()
	if err != nil {
		return err
	}

	keepIDs := make(map[int64]bool)
	for _, j := range jobs {
		keepIDs[j.ID] = true
		if !j.Enabled {
			removeCronEntry(j.ID)
			continue
		}
		if e, ok := cronEntries[j.ID]; ok {
			serverCmd.Remove(e.id)
			delete(cronEntries, j.ID)
		}
		if err := addCronJob(j); err != nil {
			appLogf("add cron job %d (%s): %v", j.ID, j.Name, err)
		}
	}
	for id := range cronEntries {
		if !keepIDs[id] {
			removeCronEntry(id)
		}
	}
	return nil
}

func addCronJob(j Job) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	if _, err := parser.Parse(j.Spec); err != nil {
		return fmt.Errorf("解析 cron 表达式失败: %w", err)
	}
	jobID := j.ID
	id, err := serverCmd.AddFunc(j.Spec, func() {
		executeJob(jobID, TriggerScheduled)
	})
	if err != nil {
		return err
	}
	cronEntries[jobID] = cronEntry{jobID: jobID, id: id}
	return nil
}

func removeCronEntry(jobID int64) {
	if e, ok := cronEntries[jobID]; ok {
		serverCmd.Remove(e.id)
		delete(cronEntries, jobID)
	}
}

func cronNextRun(jobID int64) string {
	e, ok := cronEntries[jobID]
	if !ok {
		return ""
	}
	entry := serverCmd.Entry(e.id)
	if entry.Next.IsZero() {
		return ""
	}
	return entry.Next.Format("2006-01-02 15:04:05")
}
