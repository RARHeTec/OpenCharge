// Command gateway is the MVP composition root: it hosts the YunKuaiChong TCP
// server, the settlement engine, and the admin HTTP API in one process so a
// device bill flows all the way to a persisted order and a wallet debit.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-common/money"
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/integration"
	adminapi "github.com/RARHeTec/OpenCharge/charging-core-service/module-admin"
	billing "github.com/RARHeTec/OpenCharge/charging-core-service/module-order-billing"
	"github.com/RARHeTec/OpenCharge/charging-core-service/storage"
)

func main() {
	addr := flag.String("addr", ":8768", "TCP listen address for charging piles")
	adminAddr := flag.String("admin", ":8081", "admin HTTP API address")
	tenantID := flag.String("tenant", "T1", "tenant id for demo bills")
	stationID := flag.String("station", "S1", "station id for demo bills")
	userID := flag.String("user", "U1", "wallet user id for demo bills")
	tariffID := flag.String("tariff", "V1", "platform tariff snapshot id")
	flag.Parse()

	// Shared in-memory stores (single process -> admin sees settlements live).
	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()
	seed(tariffs, wallets)

	settlement := billing.NewSettlementService(orders, tariffs, wallets, audits)
	base := integration.NewBridge(&nopSink{}, lock.NewMemoryLocker())
	bridge := integration.NewSettlementBridge(base, settlement, integration.BillConfig{
		TenantID: *tenantID, StationID: *stationID, UserID: *userID, TariffVersionID: *tariffID,
	})

	// Admin HTTP API reads the same stores.
	mux := http.NewServeMux()
	adminapi.NewAPI(audits, orders).Register(mux)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	adminSrv := &http.Server{Addr: *adminAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("admin API listening", "addr", *adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin API failed", "err", err)
		}
	}()

	gw := ykcserver.New(*addr, bridge)
	if err := gw.Start(); err != nil {
		slog.Error("gateway failed", "err", err)
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("draining")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = gw.Shutdown(ctx)
	_ = adminSrv.Shutdown(ctx)
	slog.Info("gateway stopped")
}

func seed(tariffs storage.TariffStore, wallets storage.WalletStore) {
	ctx := context.Background()
	_ = tariffs.Save(ctx, storage.TariffVersion{
		ID: "V1", TenantID: "T1", StationID: "S1", Effective: time.Now(),
		Slots: []storage.TariffSlot{
			{Slot: 0, Price: money.MustParsePrice("0.30000")}, // valley
			{Slot: 1, Price: money.MustParsePrice("0.60000")}, // peak
			{Slot: 2, Price: money.MustParsePrice("0.60000")}, // sharp (demo)
			{Slot: 3, Price: money.MustParsePrice("0.60000")}, // flat (demo)
		},
	})
	_ = wallets.Save(ctx, storage.Wallet{
		UserID: "U1", TenantID: "T1", Principal: money.MustParseYuan("10"),
	})
	slog.Info("seeded demo tariff V1 and wallet U1 (10.0000)")
}

// nopSink discards topic events in the demo; production publishes to Kafka.
type nopSink struct{}

func (nopSink) Publish(string, string, ykcserver.Event) {}
