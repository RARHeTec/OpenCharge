// Command gateway runs the YunKuaiChong V1.6 TCP server.
// Piles connect to this server as TCP clients (default port 8768 per spec).
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
)

func main() {
	addr := flag.String("addr", ":8768", "TCP listen address for charging piles")
	flag.Parse()

	srv := server.New(*addr, nil)
	if err := srv.Start(); err != nil {
		slog.Error("failed to start gateway", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("draining: stop accepting new pile connections")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("graceful shutdown timeout", "err", err)
	}
	slog.Info("gateway stopped")
}
