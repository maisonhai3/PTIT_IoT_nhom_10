// Command demo runs the entire system in one process with nothing to install: an embedded MQTT broker, the backend,
// a simulated ESP32 and a scripted weather source. Open the printed URL to use the web UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/app"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/config"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/devicesim"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/embedbroker"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/mqttx"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/store"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/weather"
)

func main() {
	var (
		httpAddr  = flag.String("http", "127.0.0.1:8080", "address for the web UI and API")
		mqttAddr  = flag.String("mqtt", "127.0.0.1:1883", "address of the embedded MQTT broker (also usable by mosquitto_sub)")
		static    = flag.String("static", "", "front-end directory (default: auto-detect ../frontend or ./frontend)")
		rainCycle = flag.Duration("rain-cycle", 4*time.Minute, "period of the scripted weather (clear -> rain expected -> raining); 0 = always clear")
		fast      = flag.Bool("fast", true, "short device timers (rain 3s, dry 20s) so the demo reacts quickly")
		live      = flag.Bool("live-weather", false, "use the real Open-Meteo API instead of the scripted weather (needs Internet)")
		token     = flag.String("token", "", "require this bearer token for POST /api/command")
		debug     = flag.Bool("debug", false, "verbose logging")
	)
	flag.Parse()

	lvl := slog.LevelInfo
	if *debug {
		lvl = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	broker, err := embedbroker.Start(*mqttAddr, nil)
	if err != nil {
		fatal(log, err)
	}
	defer broker.Close()

	env := map[string]string{
		"HTTP_ADDR": *httpAddr, "MQTT_URL": broker.URL(), "DB_PATH": ":memory:", "API_TOKEN": *token,
		"WEATHER_PUBLISH_INTERVAL": "10s", "HISTORY_INTERVAL": "5s", "STATIC_DIR": *static,
		"WEATHER_POLL_INTERVAL": "1m",
	}
	cfg, err := config.Load(func(k string) string { return env[k] })
	if err != nil {
		fatal(log, err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		fatal(log, err)
	}
	defer st.Close()

	var wx weather.Client = &weather.Scripted{Location: "Demo (thời tiết giả lập)", Period: *rainCycle, Origin: time.Now()}
	if *live {
		wx = &weather.OpenMeteo{Lat: cfg.WeatherLat, Lon: cfg.WeatherLon, Location: cfg.WeatherLocation, HTTP: &http.Client{Timeout: 15 * time.Second}}
	}

	mq := mqttx.NewPaho(mqttx.Options{URL: broker.URL(), ClientID: "awning-backend", Log: log})
	defer mq.Close()
	a := app.New(cfg, app.Deps{MQTT: mq, Weather: wx, Store: st}, log)
	if err := a.Start(ctx); err != nil {
		fatal(log, err)
	}
	mq.Connect()

	simCfg := devicesim.DefaultConfig()
	if *fast {
		simCfg = devicesim.FastConfig()
	}
	sim := devicesim.New(devicesim.Options{
		BrokerURL: broker.URL(), Prefix: cfg.TopicPrefix, Brain: simCfg,
		Every: time.Second, Seed: time.Now().UnixNano(), Log: log.With("who", "device-sim"),
	})
	go sim.Run(ctx)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: a.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fatal(log, err)
		}
	}()

	fmt.Printf("\n  Giàn phơi thông minh (demo)\n  Web UI : http://%s\n  MQTT   : %s (không mật khẩu)\n  Static : %s\n  Thoát  : Ctrl+C\n\n",
		cfg.HTTPAddr, broker.URL(), orAuto(cfg.StaticDir))

	<-ctx.Done()
	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(sctx)
}

func orAuto(s string) string {
	if s == "" {
		return "(không tìm thấy thư mục frontend/, chỉ có API)"
	}
	return s
}

func fatal(log *slog.Logger, err error) {
	log.Error("demo cannot start", "err", err)
	os.Exit(1)
}
