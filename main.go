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

	"sensing-player/internal/config"
	"sensing-player/internal/player"
	"sensing-player/internal/sensor"
	"sensing-player/internal/server"
)

func main() {
	var (
		configPath = flag.String("config", "config.yaml", "Path to config.yaml")
		logLevel   = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		noBrowser  = flag.Bool("no-browser", false, "Skip launching Chrome (useful for testing)")
	)
	flag.Parse()

	logger := newLogger(*logLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// --- HTTP server ---
	srv := server.New(cfg, logger)

	httpServer := &http.Server{
		Addr:    cfg.ServerAddr(),
		Handler: srv.Handler(),
	}

	go func() {
		logger.Info("starting HTTP server", "addr", cfg.ServerAddr())
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "err", err)
			cancel()
		}
	}()

	// Give the server a moment to bind
	time.Sleep(200 * time.Millisecond)

	// --- Chrome ---
	chromePlayer := player.New(
		cfg.Chrome.ExecutablePath,
		cfg.Chrome.KioskMode,
		cfg.Chrome.Flags,
		logger,
	)

	if !*noBrowser {
		if err := chromePlayer.Open(ctx, cfg.ServerURL()); err != nil {
			logger.Warn("failed to open Chrome", "err", err)
		}
	}

	// --- Sensor polling ---
	sensorClient := sensor.New(
		cfg.Sensor.Endpoint,
		cfg.Sensor.PollInterval,
		cfg.Sensor.TimeoutSeconds,
		logger,
	)

	readings := sensorClient.Poll(ctx)

	logger.Info("sensing-player started",
		"server", cfg.ServerURL(),
		"sensor", cfg.Sensor.Endpoint,
		"poll_interval", cfg.Sensor.PollInterval,
	)

	// --- Main loop: map sensor readings to ads ---
	for {
		select {
		case <-ctx.Done():
			shutdown(httpServer, chromePlayer, logger)
			return

		case reading, ok := <-readings:
			if !ok {
				shutdown(httpServer, chromePlayer, logger)
				return
			}
			key := selectAd(cfg, reading)
			srv.SetAd(key)
		}
	}
}

// selectAd picks an ad key based on the sensor reading.
// Rules are evaluated in order; the first match wins.
// Falls back to DefaultAdKey if no rule matches.
func selectAd(cfg *config.Config, r sensor.Reading) string {
	for _, rule := range cfg.Rules {
		minOk := rule.MinPeople < 0 || r.PeopleCount >= rule.MinPeople
		maxOk := rule.MaxPeople < 0 || r.PeopleCount <= rule.MaxPeople
		if minOk && maxOk {
			return rule.AdKey
		}
	}
	return cfg.DefaultAdKey
}

func shutdown(httpServer *http.Server, chromePlayer *player.Player, logger *slog.Logger) {
	logger.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Warn("HTTP server shutdown error", "err", err)
	}

	chromePlayer.Kill()
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(handler)
}

func init() {
	// Print usage on -help
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "sensing-player - display video ads based on WiFi sensor readings\n\n")
		fmt.Fprintf(os.Stderr, "Usage: sensing-player [flags]\n\n")
		flag.PrintDefaults()
	}
}
