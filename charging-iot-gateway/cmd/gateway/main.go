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
	"strings"
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
	adminKeys := flag.String("admin-key", "", "comma-separated keyId:tenantId pairs; empty disables auth (dev only)")
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
	registry := storage.NewMemoryRegistryStore()
	seed(tariffs, wallets)
	seedRegistry(registry, *tenantID, *stationID, *userID)

	settlement := billing.NewSettlementService(orders, tariffs, wallets, audits)
	eventBus := openBus(*kafkaBroker)
	defer eventBus.Close()
	base := integration.NewBridge(&busSink{bus: eventBus}, lock.NewMemoryLocker())
	settlementBridge := integration.NewSettlementBridge(base, settlement, integration.BillConfig{
		TenantID: *tenantID, StationID: *stationID, UserID: *userID,
		TariffVersionID: *tariffID,
		TariffAt:        tariffAtResolver{store: tariffs},
		Location:        time.Local,
		Resolver:        registryResolver{store: registry},
	})
	stateStore := openStateStore(*redisAddr)
	_, telemetryBuffer := openTelemetryStore(*tdengine)
	defer telemetryBuffer.Close()
	stateHook := integration.NewStateHook(stateStore, telemetryBuffer)
	bridge := integration.CompositeHandler{stateHook, settlementBridge}

	// Admin HTTP API reads the same stores.
	mux := http.NewServeMux()
	api := adminapi.NewAPI(audits, orders, stateStore).
		WithTariffs(tariffs).WithResolver(settlement).WithRegistry(registry).
		WithKeys(parseAdminKeys(*adminKeys))
	if *adminKeys != "" {
		slog.Info("admin API authentication enabled")
	} else {
		slog.Warn("admin API authentication disabled (dev mode); pass -admin-key in production")
	}
	api.Register(mux)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	adminSrv := &http.Server{Addr: *adminAddr, Handler: api.AuthMiddleware(mux), ReadHeaderTimeout: 5 * time.Second}
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
	// Deep-night valley 0.30 for slots 0-1 (00:00-01:00), 0.60 for the rest.
	// This matches the simulator's default 0.60000 bill so the demo settles
	// exactly; edit prices in the admin UI to exercise the exception path.
	slots := make([]storage.TariffSlot, 48)
	for i := range slots {
		price := "0.60000"
		if i < 2 {
			price = "0.30000"
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

// tariffAtResolver picks the tariff snapshot effective at a moment so delayed
// offline bills are priced with the rate that was live during charging.
type tariffAtResolver struct {
	store storage.TariffStore
}

func (r tariffAtResolver) ResolveTariffVersion(ctx context.Context, tenantID, stationID string, at time.Time) (string, error) {
	v, ok, err := r.store.Latest(ctx, tenantID, stationID, at.UnixMilli())
	if err != nil || !ok {
		return "", err
	}
	slog.Info("resolved tariff for charge time", "tariff", v.ID, "at", at.Format(time.RFC3339))
	return v.ID, nil
}

// parseAdminKeys turns "key1:T1,key2:T2" into a key registry. An empty string
// returns nil, which disables auth for local single-tenant use.
func parseAdminKeys(spec string) adminapi.KeyRegistry {
	if spec == "" {
		return nil
	}
	pairs := make(map[string]adminapi.Principal)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 || kv[0] == "" || kv[1] == "" {
			slog.Warn("ignoring malformed admin key entry", "entry", part)
			continue
		}
		pairs[kv[0]] = adminapi.Principal{KeyID: kv[0], TenantID: kv[1]}
	}
	if len(pairs) == 0 {
		return nil
	}
	return adminapi.NewStaticKeys(pairs)
}

// registryResolver looks up a device's billing context from master data so the
// gateway can serve multiple stations and tenants without per-process flags.
type registryResolver struct {
	store storage.RegistryStore
}

func (r registryResolver) ResolveDevice(ctx context.Context, deviceID string) (integration.DeviceBilling, bool) {
	d, ok, err := r.store.GetDevice(ctx, deviceID)
	if err != nil || !ok {
		return integration.DeviceBilling{}, false
	}
	return integration.DeviceBilling{
		TenantID: d.TenantID, StationID: d.StationID, UserID: d.DefaultUserID,
	}, true
}

// seedRegistry registers the demo station and a device so the simulator's
// default pile id resolves without manual setup.
func seedRegistry(reg storage.RegistryStore, tenantID, stationID, userID string) {
	ctx := context.Background()
	_ = reg.SaveStation(ctx, storage.Station{
		ID: stationID, TenantID: tenantID, Name: "演示场站", Timezone: "Asia/Shanghai",
		CreatedAt: time.Now(),
	})
	for _, id := range []string{"55031412782305", "55031412782306", "55031412782307",
		"55031412782310", "55031412782311", "55031412782312", "55304121873250"} {
		_ = reg.SaveDevice(ctx, storage.Device{
			DeviceID: id, TenantID: tenantID, StationID: stationID,
			Protocol: "yunkuaichong-v1.6", DefaultUserID: userID, GunCount: 2,
		})
	}
	slog.Info("seeded demo station and devices", "station", stationID, "tenant", tenantID)
}
