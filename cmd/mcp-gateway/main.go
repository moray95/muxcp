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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gw := gateway.NewGateway(cfg)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down...")
		gw.Shutdown()
		cancel()
		os.Exit(0)
	}()

	if err := gw.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "gateway error: %v\n", err)
		os.Exit(1)
	}
}
