package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func findBin(name string) string {
	candidates := []string{
		filepath.Join(appDest, "app", name),
		filepath.Join(appDest, name),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			if mode := st.Mode(); mode&0o111 != 0 {
				return p
			}
		}
	}
	return ""
}

func appLogf(format string, args ...any) {
	ts := timeNow().Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s", ts, fmt.Sprintf(format, args...))
	log.Print(line)
}
