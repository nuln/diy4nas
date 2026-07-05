package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]

	cmd := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			cmd = a
			break
		}
	}

	switch cmd {
	case "status":
		resp := map[string]any{
			"BackendState": "Running",
			"Self": map[string]any{
				"ID":           "self",
				"PublicKey":    "mock:publickey",
				"TailscaleIPs": []string{"100.64.0.1"},
				"DNSName":      "test.tailnet",
				"HostName":     "test",
				"Online":       true,
				"Active":       true,
				"OS":           "linux",
			},
			"Peer":    map[string]any{},
			"User":    map[string]any{"1": map[string]any{"LoginName": "test@example.com"}},
			"Version": "1.98.4",
		}
		json.NewEncoder(os.Stdout).Encode(resp)
	case "up", "down", "logout", "set", "ping", "netcheck":
		// silently succeed
	default:
		if cmd != "" {
			fmt.Fprintf(os.Stderr, "mock tailscale: unknown command %q\n", cmd)
		}
	}
}
