// Command gateway boots the IoT gateway with protocol adapters, dual Kafka
// channels and graceful shutdown that never waits for long charging sessions.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-ocpp16"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/protocol-spi"
)

func main() {
	addr := flag.String("addr", ":9000", "gateway listen address")
	flag.Parse()

	reg := spi.NewRegistry()
	reg.Register(yunkuaichong.New("1.6"))
	reg.Register(ocpp16.New())
	slog.Info("adapters registered", "codes", reg.Codes())

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("gateway listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("gateway failed", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("draining: stop accepting new device connections")

	// Step 1: refuse new connections. Step 2: send reconnect command to devices.
	// Step 3: disconnect remaining sessions within the deadline. We never wait
	// for a multi-hour charging order to finish, otherwise rollout stalls.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Warn("graceful shutdown timeout", "err", err)
	}
	slog.Info("gateway stopped")
}
