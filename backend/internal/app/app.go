// Package app wires the backend together. cmd/server, cmd/demo and the end-to-end tests all build it the same way,
// differing only in which MQTT client, weather source and store they plug in.
package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/api"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/config"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/hub"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/service"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/weather"
)

type Deps struct {
	MQTT    mqttx.Client
	Weather weather.Client
	Store   store.Store
}

type App struct {
	Service *service.Service
	Hub     *hub.Hub
	API     *api.Server

	poller *weather.Poller
	log    *slog.Logger
}

func New(cfg config.Config, d Deps, log *slog.Logger) *App {
	h := hub.New(log)
	svc := service.New(service.Options{
		Prefix:                 cfg.TopicPrefix,
		DeviceTimeout:          cfg.DeviceTimeout,
		WeatherPublishInterval: cfg.WeatherPublishInterval,
		HistoryInterval:        cfg.HistoryInterval,
		HistoryRetention:       cfg.HistoryRetention,
		Log:                    log,
	}, d.MQTT, d.Store, h)
	a := &App{Service: svc, Hub: h, log: log}
	a.API = api.New(svc, h, api.Options{
		Token: cfg.APIToken, CORSOrigins: cfg.CORSOrigins, StaticDir: cfg.StaticDir, Log: log,
	})
	a.poller = &weather.Poller{
		Client:   d.Weather,
		Interval: cfg.WeatherPollInterval,
		OnUpdate: svc.SetWeather,
		OnError:  svc.RecordWeatherError,
		Log:      log,
	}
	return a
}

func (a *App) Handler() http.Handler { return a.API.Handler() }

// Start subscribes to MQTT and launches the background loops (device watchdog, weather poller). It returns
// immediately; everything stops when ctx is cancelled.
func (a *App) Start(ctx context.Context) error {
	if err := a.Service.Start(ctx); err != nil {
		return err
	}
	go a.poller.Run(ctx)
	return nil
}
