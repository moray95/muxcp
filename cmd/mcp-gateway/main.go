package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/moray/mcp-gateway/internal/gateway"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to gateway config file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := gateway.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if cfg.Transport == gateway.TransportStdio {
		slog.Info("loaded config", "servers", len(cfg.Servers), "transport", cfg.Transport)
	} else {
		slog.Info("loaded config", "servers", len(cfg.Servers), "listen", cfg.Listen, "transport", cfg.Transport)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	gw := gateway.NewGateway(cfg)

	if err := gw.Start(ctx); err != nil {
		gw.Shutdown()
		cancel()
		slog.Error("gateway error", "error", err)
		os.Exit(1) //nolint:gocritic // intentional exit on startup failure
	}

	<-ctx.Done()
	slog.Info("shutting down...")
	gw.Shutdown()
}
