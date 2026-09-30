package main

import (
	"net"
	"net/http"
	"time"
)

// healthcheck asks the local server for /healthz and returns a process exit code. It exists because the runtime
// image is distroless (no shell, no curl), so Docker's HEALTHCHECK has to run this binary itself.
func healthcheck(httpAddr string) int {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	c := http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
