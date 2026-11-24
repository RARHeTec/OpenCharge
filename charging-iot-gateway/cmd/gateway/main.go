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
	ykcserver "github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/server"
	"github.com/RARHeTec/OpenCharge/charging-common/money"
	"github.com/RARHeTec/OpenCharge/charging-common/state"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/telemetry"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/bus"
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
	redisAddr := flag.String("redis", "", "Redis address for real-time connector state; empty = memory")
	tdengine := flag.String("tdengine", "", "TDengine REST endpoint (http://host:6041); empty = memory")
	kafkaBroker := flag.String("kafka", "", "Kafka broker host:port; empty = in-process bus")
	flag.Parse()

	// Shared in-memory stores (single process -> admin sees settlements live).
	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()
	seed(tariffs, wallets)

	settlement := billing.NewSettlementService(orders, tariffs, wallets, audits)
	eventBus := openBus(*kafkaBroker)
	defer eventBus.Close()
	base := integration.NewBridge(&busSink{bus: eventBus}, lock.NewMemoryLocker())
	settlementBridge := integration.NewSettlementBridge(base, settlement, integration.BillConfig{
		TenantID: *tenantID, StationID: *stationID, UserID: *userID, TariffVersionID: *tariffID,
	})
	stateStore := openStateStore(*redisAddr)
	_, telemetryBuffer := openTelemetryStore(*tdengine)
	defer telemetryBuffer.Close()
	stateHook := integration.NewStateHook(stateStore, telemetryBuffer)
	bridge := integration.CompositeHandler{stateHook, settlementBridge}

	// Admin HTTP API reads the same stores.
	mux := http.NewServeMux()
	adminapi.NewAPI(audits, orders, stateStore).WithTariffs(tariffs).Register(mux)
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
	// Demo snapshot fills all 48 half-hour slots so it satisfies the same
	// validation rule enforced by the admin API and never fails repricing.
	// Valley 0.30 for slots 0-15, flat 0.60 for 16-23, peak 0.60 for 24-47.
	slots := make([]storage.TariffSlot, 48)
	for i := range slots {
		price := "0.60000"
		switch {
		case i < 16:
			price = "0.30000"
		case i < 24:
			price = "0.60000"
		default:
			price = "0.60000"
		}
		slots[i] = storage.TariffSlot{Slot: i, Price: money.MustParsePrice(price)}
	}
	_ = tariffs.Save(ctx, storage.TariffVersion{
		ID: "V1", TenantID: "T1", StationID: "S1", Effective: time.Now(), Slots: slots,
	})
	_ = wallets.Save(ctx, storage.Wallet{
		UserID: "U1", TenantID: "T1", Principal: money.MustParseYuan("10"),
	})
	slog.Info("seeded demo tariff V1 and wallet U1 (10.0000)")
}

// nopSink discards topic events in the demo; production publishes to Kafka.
type nopSink struct{}

func (nopSink) Publish(string, string, ykcserver.Event) {}

func openStateStore(addr string) state.Store {
	if addr == "" {
		slog.Info("using in-memory connector state")
		return state.NewMemoryStore()
	}
	rdb := state.NewRedisClient(addr)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Warn("Redis unreachable, falling back to memory", "addr", addr, "err", err)
		return state.NewMemoryStore()
	}
	slog.Info("using Redis connector state", "addr", addr)
	return state.NewRedisStore(rdb, 5*time.Minute)
}

func openTelemetryStore(endpoint string) (telemetry.Store, *telemetry.Buffer) {
	if endpoint == "" {
		slog.Info("using in-memory telemetry store")
		mem := telemetry.NewMemoryStore()
		return mem, telemetry.NewBuffer(mem, 50, 3*time.Second)
	}
	td := telemetry.NewTDengineStore(endpoint, "opencharge")
	if err := td.Init(context.Background()); err != nil {
		slog.Warn("TDengine unreachable, falling back to memory", "err", err)
		mem := telemetry.NewMemoryStore()
		return mem, telemetry.NewBuffer(mem, 50, 3*time.Second)
	}
	slog.Info("using TDengine telemetry store", "endpoint", endpoint)
	return td, telemetry.NewBuffer(td, 50, 3*time.Second)
}

func openBus(broker string) bus.Bus {
	if broker == "" {
		slog.Info("using in-process event bus")
		return bus.NewMemoryBus()
	}
	slog.Info("using Kafka event bus", "broker", broker)
	return bus.NewKafkaBus(broker)
}

// busSink adapts a Bus to the Sink interface used by the legacy Bridge.
type busSink struct{ bus bus.Bus }

func (s *busSink) Publish(topic, key string, e ykcserver.Event) {
	if err := s.bus.Publish(context.Background(), topic, key, e); err != nil {
		slog.Warn("event publish failed", "topic", topic, "err", err)
	}
}
