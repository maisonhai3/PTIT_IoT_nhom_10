package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckExitCodes(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ok.Close()
	if code := healthcheck(strings.TrimPrefix(ok.URL, "http://")); code != 0 {
		t.Errorf("healthy server: exit %d, want 0", code)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 500) }))
	defer bad.Close()
	if code := healthcheck(strings.TrimPrefix(bad.URL, "http://")); code != 1 {
		t.Errorf("failing server: exit %d, want 1", code)
	}

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if code := healthcheck(addr); code != 1 {
		t.Errorf("nothing listening: exit %d, want 1", code)
	}
	if code := healthcheck("not-an-address"); code != 1 {
		t.Errorf("garbage address: exit %d, want 1", code)
	}
}

func TestHealthcheckWildcardHostBecomesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	for _, host := range []string{"", "0.0.0.0"} {
		if code := healthcheck(net.JoinHostPort(host, port)); code != 0 {
			t.Errorf("HTTP_ADDR host %q: exit %d, want 0", host, code)
		}
	}
}
