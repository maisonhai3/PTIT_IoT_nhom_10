// Command server runs the awning backend: MQTT <-> REST/WebSocket, weather polling, history, and the web UI.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/app"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/config"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/weather"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// .env is a convenience for local runs; real environment variables always win.
	for _, p := range []string{".env", "backend/.env"} {
		if err := config.LoadDotEnv(p); err != nil {
			return err
		}
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	mq := mqttx.NewPaho(mqttx.Options{
		URL: cfg.MQTTURL, ClientID: cfg.MQTTClientID, Username: cfg.MQTTUser, Password: cfg.MQTTPassword, Log: log,
	})
	defer mq.Close()

	a := app.New(cfg, app.Deps{
		MQTT: mq,
		Weather: &weather.OpenMeteo{
			Lat: cfg.WeatherLat, Lon: cfg.WeatherLon, Location: cfg.WeatherLocation,
			HTTP: &http.Client{Timeout: 15 * time.Second},
		},
		Store: st,
	}, log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.Start(ctx); err != nil {
		return err
	}
	mq.Connect()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	staticNote := cfg.StaticDir
	if staticNote == "" {
		staticNote = "(not serving the front-end: no frontend/ directory found, set STATIC_DIR)"
	}
	log.Info("backend started", "http", cfg.HTTPAddr, "mqtt", cfg.MQTTURL, "topic_prefix", cfg.TopicPrefix,
		"static", staticNote, "weather_at", [2]float64{cfg.WeatherLat, cfg.WeatherLon}, "token_required", cfg.APIToken != "")

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
