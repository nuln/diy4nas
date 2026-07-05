package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	sockPath := os.Getenv("TAILSCALE_SOCKET")
	if sockPath == "" {
		sockPath = "/tmp/tailscaled.sock"
	}

	os.Remove(sockPath)
	os.MkdirAll("/tmp", 0755)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mock tailscaled: listen error: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "mock tailscaled: listening on %s\n", sockPath)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	envFile := sockPath + ".env"
	envData := strings.Join(os.Environ(), "\n") + "\n"
	os.WriteFile(envFile, []byte(envData), 0644)
	fmt.Fprintf(os.Stderr, "mock tailscaled: env saved to %s\n", envFile)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	fmt.Fprintf(os.Stderr, "mock tailscaled: shutting down\n")
	ln.Close()
	os.Remove(sockPath)
	os.Remove(envFile)
}
