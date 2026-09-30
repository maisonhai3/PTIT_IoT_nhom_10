// Package api exposes the REST + WebSocket interface described in docs/openapi.yaml and serves the front-end.
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/model"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/service"
)

// Backend is what the handlers need; *service.Service implements it.
type Backend interface {
	State() model.State
	Weather() (model.Weather, bool)
	SendCommand(a model.Action) error
	History(ctx context.Context, hours int) ([]model.HistoryPoint, error)
	Events(ctx context.Context, limit int) ([]model.Event, error)
	MQTTConnected() bool
	DeviceOnline() bool
}

type Options struct {
	Token       string   // empty: POST /api/command needs no token
	CORSOrigins []string // allowed cross-origin browser origins; "*" for any
	StaticDir   string   // empty: do not serve the front-end
	Log         *slog.Logger
}

type Server struct {
	be  Backend
	hub *hub.Hub
	o   Options
}

func New(be Backend, h *hub.Hub, o Options) *Server {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Server{be: be, hub: h, o: o}
}

// apiRoutes lists the API paths and their methods, used to answer 405 vs 404 in JSON.
var apiRoutes = map[string]string{
	"/api/state":   "GET",
	"/api/command": "POST",
	"/api/weather": "GET",
	"/api/history": "GET",
	"/api/events":  "GET",
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.getState)
	mux.HandleFunc("POST /api/command", s.postCommand)
	mux.HandleFunc("GET /api/weather", s.getWeather)
	mux.HandleFunc("GET /api/history", s.getHistory)
	mux.HandleFunc("GET /api/events", s.getEvents)
	mux.HandleFunc("GET /healthz", s.getHealth)
	mux.Handle("GET /ws", s.hub.Handler(s.wsOrigins(), s.initialMessages))
	mux.HandleFunc("/api/", s.apiFallback)
	if s.o.StaticDir != "" {
		// Registered without a method so it does not conflict with the "/api/" fallback (Go 1.22 mux rules).
		mux.Handle("/", s.static())
	}
	return s.recoverer(s.logRequests(s.cors(mux)))
}

// ---- handlers ----

func (s *Server) getState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.be.State())
}

func (s *Server) getWeather(w http.ResponseWriter, _ *http.Request) {
	wx, ok := s.be.Weather()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "weather not available yet")
		return
	}
	writeJSON(w, http.StatusOK, wx)
}

func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	hours, ok := intParam(w, r, "hours", 24, 1, 168)
	if !ok {
		return
	}
	pts, err := s.be.History(r.Context(), hours)
	if err != nil {
		s.o.Log.Error("history query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pts == nil {
		pts = []model.HistoryPoint{}
	}
	writeJSON(w, http.StatusOK, pts)
}

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	limit, ok := intParam(w, r, "limit", 50, 1, 200)
	if !ok {
		return
	}
	evs, err := s.be.Events(r.Context(), limit)
	if err != nil {
		s.o.Log.Error("events query failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if evs == nil {
		evs = []model.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) getHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"mqtt_connected": s.be.MQTTConnected(),
		"device_online":  s.be.DeviceOnline(),
	})
}

func (s *Server) postCommand(w http.ResponseWriter, r *http.Request) {
	// A plain HTML form or fetch(no-cors) can send text/plain cross-site without a CORS preflight, so requiring
	// JSON (which forces a preflight) plus an Origin check stops other web pages from driving the awning.
	if !s.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="awning"`)
		writeError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}
	if ct := strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]); !strings.EqualFold(ct, "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body struct {
		Action model.Action `json:"action"`
	}
	if err := dec.Decode(&body); err != nil || dec.More() {
		writeError(w, http.StatusBadRequest, `body must be {"action": "..."}`)
		return
	}
	if !body.Action.Valid() { // validate at the boundary; the service checks again as it is also a public entry point
		writeError(w, http.StatusBadRequest, "unknown action; use open, close, auto, simulate_rain or clear_rain")
		return
	}
	switch err := s.be.SendCommand(body.Action); {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "sent"})
	case errors.Is(err, service.ErrInvalidAction):
		writeError(w, http.StatusBadRequest, "unknown action; use open, close, auto, simulate_rain or clear_rain")
	case errors.Is(err, service.ErrDeviceOffline):
		writeError(w, http.StatusConflict, "device is offline")
	case errors.Is(err, service.ErrMQTT):
		writeError(w, http.StatusServiceUnavailable, "mqtt broker unavailable")
	default:
		s.o.Log.Error("command failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *Server) apiFallback(w http.ResponseWriter, r *http.Request) {
	if method, known := apiRoutes[r.URL.Path]; known {
		w.Header().Set("Allow", method)
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) initialMessages() []hub.Message {
	msgs := []hub.Message{{Type: "state", Data: s.be.State()}}
	if wx, ok := s.be.Weather(); ok {
		msgs = append(msgs, hub.Message{Type: "weather", Data: wx})
	}
	return msgs
}

// ---- static front-end ----

func (s *Server) static() http.Handler {
	files := http.FileServer(http.Dir(s.o.StaticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		// The front-end has no inline script or style: everything is same-origin.
		h.Set("Content-Security-Policy",
			"default-src 'self'; connect-src 'self' ws: wss:; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("Cache-Control", "no-cache") // revalidate (ETag) so edits show up on reload
		files.ServeHTTP(w, r)
	})
}

// ---- middleware ----

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				s.o.Log.Error("panic in handler", "panic", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the real writer.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" { // WebSocket: do not wrap the writer, the upgrade needs the raw one
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		lvl := slog.LevelDebug
		switch {
		case rec.status == http.StatusInternalServerError:
			lvl = slog.LevelError
		case rec.status >= 400: // includes the normal "503: no weather yet" and client mistakes
			lvl = slog.LevelWarn
		}
		s.o.Log.Log(r.Context(), lvl, "http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"dur_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !sameOrigin(r, origin) && s.corsAllowed(origin) {
			h := w.Header()
			if s.allowAnyOrigin() {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) allowAnyOrigin() bool {
	for _, o := range s.o.CORSOrigins {
		if o == "*" {
			return true
		}
	}
	return false
}

func (s *Server) corsAllowed(origin string) bool {
	if s.allowAnyOrigin() {
		return true
	}
	for _, o := range s.o.CORSOrigins {
		if strings.EqualFold(strings.TrimRight(o, "/"), origin) {
			return true
		}
	}
	return false
}

func sameOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// originAllowed: requests without an Origin header (curl, scripts) pass; browsers must come from this
// server's own pages or an explicitly allowed origin.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || sameOrigin(r, origin) {
		return true
	}
	return s.corsAllowed(origin)
}

func (s *Server) authorized(r *http.Request) bool {
	if s.o.Token == "" {
		return true
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(s.o.Token)) == 1
}

// wsOrigins converts CORS origins ("http://host:port") to the host patterns the WebSocket library expects.
func (s *Server) wsOrigins() []string {
	var out []string
	for _, o := range s.o.CORSOrigins {
		if o == "*" {
			return []string{"*"}
		}
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
		} else {
			out = append(out, o)
		}
	}
	return out
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func intParam(w http.ResponseWriter, r *http.Request, name string, def, lo, hi int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < lo || n > hi {
		writeError(w, http.StatusBadRequest, name+" must be an integer between "+strconv.Itoa(lo)+" and "+strconv.Itoa(hi))
		return 0, false
	}
	return n, true
}
