// Command server boots the modular monolith.
// Without -dsn it runs an in-memory demo store so the whole platform can be
// exercised locally; with -dsn it uses PostgreSQL.
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

	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-common/trace"
	"github.com/RARHeTec/OpenCharge/charging-core-service/module-admin"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func main() {
	addr := flag.String("addr", ":8080", "server listen address")
	dsn := flag.String("dsn", "", "PostgreSQL DSN; empty = in-memory demo mode")
	flag.Parse()

	orders, _, _, audits := openStores(*dsn)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx := trace.WithTrace(r.Context(), trace.FromHTTPHeader(r.Header.Values(trace.HTTPHeader)))
		slog.Info("healthz", "traceId", trace.FromContext(ctx))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	admin.NewAPI(audits, orders).Register(mux)
	slog.Info("admin API mounted", "dsn", maskDSN(*dsn))

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("core server listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("server stopped")
}

func openStores(dsn string) (storage.OrderStore, storage.TariffStore, storage.WalletStore, storage.AuditStore) {
	if dsn != "" {
		pg, err := storage.OpenPostgres(dsn)
		if err != nil {
			slog.Error("postgres unavailable", "err", err)
			os.Exit(1)
		}
		if err := pg.AutoMigrate(); err != nil {
			slog.Error("auto migrate failed", "err", err)
			os.Exit(1)
		}
		return &storage.PGOrderStore{S: pg}, &storage.PGTariffStore{S: pg},
			&storage.PGWalletStore{S: pg}, &storage.PGAuditStore{S: pg}
	}

	// In-memory demo stores: seed one tariff and one wallet for local testing.
	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()
	ctx := context.Background()
	_ = tariffs.Save(ctx, storage.TariffVersion{
		ID: "V1", TenantID: "T1", StationID: "S1", Effective: time.Now(),
		Slots: []storage.TariffSlot{
			{Slot: 0, Price: money.MustParsePrice("0.30000")},
			{Slot: 1, Price: money.MustParsePrice("0.60000")},
		},
	})
	_ = wallets.Save(ctx, storage.Wallet{UserID: "U1", TenantID: "T1", Principal: money.MustParseYuan("10")})
	slog.Info("using in-memory demo stores (no -dsn)")
	return orders, tariffs, wallets, audits
}

func maskDSN(dsn string) string {
	if dsn == "" {
		return "memory"
	}
	return "postgres://***"
}
