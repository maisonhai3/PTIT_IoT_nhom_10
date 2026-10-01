// Command simulator pretends to be the ESP32: it speaks the same MQTT protocol against a real broker, so the backend
// and the web UI can be exercised without hardware. Use it with the Mosquitto from deploy/.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/config"
	"github.com/maisonhai3/ptit_iot_nhom_10/backend/internal/devicesim"
)

func main() {
	_ = config.LoadDotEnv(".env")
	var (
		broker = flag.String("broker", envOr("SIM_MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
		user   = flag.String("user", envOr("SIM_MQTT_USER", "esp32-awning01"), "MQTT user (the device's account)")
		pass   = flag.String("pass", os.Getenv("SIM_MQTT_PASSWORD"), "MQTT password")
		prefix = flag.String("prefix", envOr("MQTT_TOPIC_PREFIX", "pkg/awning01/"), "topic prefix")
		fast   = flag.Bool("fast", true, "short timers (forecast rain 3s, rain plate 2s, dry 20s, trip 4s) instead of the firmware's (30s, 5s, 15min, 8s)")
		every  = flag.Duration("telemetry", 5*time.Second, "telemetry period")
	)
	flag.Parse()

	cfg := devicesim.DefaultConfig()
	if *fast {
		cfg = devicesim.FastConfig()
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("device simulator starting", "broker", *broker, "user", *user, "fast", *fast)
	sim := devicesim.New(devicesim.Options{
		BrokerURL: *broker, Username: *user, Password: *pass, Prefix: *prefix,
		Brain: cfg, Every: *every, Seed: time.Now().UnixNano(), Log: log,
	})
	if err := sim.Run(ctx); err != nil {
		log.Error("simulator stopped", "err", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
