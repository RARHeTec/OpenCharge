// Command gateway is the MVP composition root: it hosts the YunKuaiChong TCP
// server, the settlement engine, and the admin HTTP API in one process so a
// device bill flows all the way to a persisted order and a wallet debit.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/RARHeTec/OpenCharge/charging-common/heartbeat"
	"github.com/RARHeTec/OpenCharge/charging-common/lock"
	"github.com/RARHeTec/OpenCharge/charging-iot-gateway/adapter-yunkuaichong/protocol"
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
	dsn := flag.String("dsn", "", "PostgreSQL DSN; empty = in-memory demo mode (data lost on restart)")
	tenantID := flag.String("tenant", "T1", "tenant id for demo bills")
	stationID := flag.String("station", "S1", "station id for demo bills")
	userID := flag.String("user", "U1", "wallet user id for demo bills")
	tariffID := flag.String("tariff", "V1", "platform tariff snapshot id")
	redisAddr := flag.String("redis", "", "Redis address for real-time connector state; empty = memory")
	tdengine := flag.String("tdengine", "", "TDengine REST endpoint (http://host:6041); empty = memory")
	kafkaBroker := flag.String("kafka", "", "Kafka broker host:port; empty = in-process bus")
	bcdOrder := flag.String("bcd-order", "low", "device-id BCD nibble order: low (real piles) | high (spec sample/simulator)")
	flag.Parse()

	if err := protocol.SetBCDOrder(*bcdOrder); err != nil {
		slog.Error("invalid -bcd-order", "err", err)
		os.Exit(2)
	}
	slog.Info("device-id BCD order", "order", *bcdOrder)

	// All admin data lives in these stores. With -dsn they are PostgreSQL
	// backed; without it everything is in memory and lost on restart.
	orders, tariffs, wallets, audits, registry, pricePlans := openStores(
		*dsn, *tenantID, *stationID, *userID)

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
	heartbeatStore := openHeartbeatStore(*redisAddr)
	_, telemetryBuffer := openTelemetryStore(*tdengine)
	defer telemetryBuffer.Close()
	stateHook := integration.NewStateHook(stateStore, telemetryBuffer, heartbeatStore)
	// The TCP server must exist before the downlink so it can send 0x58 frames.
	gw := ykcserver.New(*addr, nil)
	downlink := integration.NewTariffDownlink(gw, registry, pricePlans, stateStore)
	bridge := integration.CompositeHandler{stateHook, settlementBridge, downlink}
	gw.SetHandler(bridge)
	gw.TariffModelFor = func(deviceID string) (protocol.TariffModel, bool) {
		return tariffModelFor(context.Background(), deviceID, registry, pricePlans)
	}

	// Admin HTTP API reads the same stores.
	mux := http.NewServeMux()
	api := adminapi.NewAPI(audits, orders, stateStore).
		WithTariffs(tariffs).WithResolver(settlement).WithRegistry(registry).
		WithHeartbeats(heartbeatStore).WithPricePlans(pricePlans).
		WithDeliverer(downlink).WithKeys(parseAdminKeys(*adminKeys))
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
	// Two device types so the admin pricing page shows both cards, matching
	// the operator's screenshot (one pile of each type).
	for _, d := range []struct{ id, deviceType string }{
		{"55031412782305", "直流160kW-浙江艾充ZJAC-DC-160KW"},
		{"55031412782306", "直流240kW-浙江艾充ZJAC-DC-240KW"},
		{"55031412782307", "直流160kW-浙江艾充ZJAC-DC-160KW"},
		{"55031412782310", "直流240kW-浙江艾充ZJAC-DC-240KW"},
		{"55031412782311", "直流160kW-浙江艾充ZJAC-DC-160KW"},
		{"55031412782312", "直流240kW-浙江艾充ZJAC-DC-240KW"},
		{"55304121873250", "直流160kW-浙江艾充ZJAC-DC-160KW"},
	} {
		_ = reg.SaveDevice(ctx, storage.Device{
			DeviceID: d.id, TenantID: tenantID, StationID: stationID,
			Protocol: "yunkuaichong-v1.6", DeviceType: d.deviceType,
			DefaultUserID: userID, GunCount: 2,
		})
	}
	slog.Info("seeded demo station and devices", "station", stationID, "tenant", tenantID)
}

// openHeartbeatStore shares the Redis connection with connector state when
// configured; counters live in their own hash keys with no TTL.
func openHeartbeatStore(addr string) heartbeat.Store {
	if addr == "" {
		return heartbeat.NewMemoryStore()
	}
	rdb := state.NewRedisClient(addr)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Warn("Redis unreachable for heartbeats, using memory", "err", err)
		return heartbeat.NewMemoryStore()
	}
	slog.Info("using Redis heartbeat counters", "addr", addr)
	return heartbeat.NewRedisStore(rdb)
}

// seedPricePlans creates the demo prices shown in the first screenshot so the
// admin page has real data on a fresh start.
func seedPricePlans(store storage.PricePlanStore, tenantID, stationID string) {
	ctx := context.Background()
	segs := []storage.PriceSegment{
		{Seq: 1, StartMin: 0, EndMin: 420, Period: storage.PeriodValley,
			ElecPrice: money.MustParsePrice("0.6"), ServicePrice: money.MustParsePrice("0.18")},
		{Seq: 2, StartMin: 420, EndMin: 660, Period: storage.PeriodFlat,
			ElecPrice: money.MustParsePrice("0.88"), ServicePrice: money.MustParsePrice("0.15")},
		{Seq: 3, StartMin: 660, EndMin: 840, Period: storage.PeriodValley,
			ElecPrice: money.MustParsePrice("0.6"), ServicePrice: money.MustParsePrice("0.18")},
		{Seq: 4, StartMin: 840, EndMin: 960, Period: storage.PeriodFlat,
			ElecPrice: money.MustParsePrice("0.88"), ServicePrice: money.MustParsePrice("0.15")},
		{Seq: 5, StartMin: 960, EndMin: 1350, Period: storage.PeriodPeak,
			ElecPrice: money.MustParsePrice("1.195"), ServicePrice: money.MustParsePrice("0.095")},
		{Seq: 6, StartMin: 1350, EndMin: 1440, Period: storage.PeriodFlat,
			ElecPrice: money.MustParsePrice("0.88"), ServicePrice: money.MustParsePrice("0.15")},
	}
	for i, dt := range []string{
		"直流160kW-浙江艾充ZJAC-DC-160KW",
		"直流240kW-浙江艾充ZJAC-DC-240KW",
	} {
		_ = store.Save(ctx, storage.PricePlan{
			ID: fmt.Sprintf("pp-demo-%d", i+1), TenantID: tenantID, StationID: stationID,
			DeviceType: dt, EffectiveAt: time.Now(), EffectiveNow: true,
			Status: "valid", Source: "manual", Operator: "138190", ModelCode: i + 1,
			Segments: segs, CreatedAt: time.Now(), DeliveredCount: 1, SuccessCount: 1,
		})
	}
	slog.Info("seeded demo price plans", "station", stationID)
}

// tariffModelFor resolves a device's active PricePlan into the 0x58 model
// returned to 0x05 verification and 0x09 model requests from the pile.
func tariffModelFor(ctx context.Context, deviceID string,
	reg storage.RegistryStore, plans storage.PricePlanStore) (protocol.TariffModel, bool) {
	dev, ok, err := reg.GetDevice(ctx, deviceID)
	if err != nil || !ok {
		return protocol.TariffModel{}, false
	}
	plan, ok, err := plans.Current(ctx, dev.TenantID, dev.StationID, dev.DeviceType)
	if err != nil || !ok {
		return protocol.TariffModel{}, false
	}
	code, rates, slots, err := storage.PriceToProtocol(plan)
	if err != nil {
		return protocol.TariffModel{}, false
	}
	return protocol.TariffModel{
		ModelCode: code,
		Rates: protocol.TariffRates{
			SharpElec: uint32(rates.SharpElec), SharpService: uint32(rates.SharpService),
			PeakElec: uint32(rates.PeakElec), PeakService: uint32(rates.PeakService),
			FlatElec: uint32(rates.FlatElec), FlatService: uint32(rates.FlatService),
			ValleyElec: uint32(rates.ValleyElec), ValleyService: uint32(rates.ValleyService),
		},
		Slots: slots,
	}, true
}

// openStores picks PostgreSQL when a DSN is supplied, otherwise the verified
// in-memory demo stores. Seeds are applied only to the in-memory mode so a
// real database keeps whatever the operator configured through the admin API.
func openStores(dsn, tenantID, stationID, userID string) (
	storage.OrderStore, storage.TariffStore, storage.WalletStore,
	storage.AuditStore, storage.RegistryStore, storage.PricePlanStore) {
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
		slog.Info("using PostgreSQL stores", "dsn", maskDSN(dsn))
		return &storage.PGOrderStore{S: pg}, &storage.PGTariffStore{S: pg},
			&storage.PGWalletStore{S: pg}, &storage.PGAuditStore{S: pg},
			&storage.PGRegistryStore{S: pg}, &storage.PGPricePlanStore{S: pg}
	}

	orders := storage.NewMemoryOrderStore()
	tariffs := storage.NewMemoryTariffStore()
	wallets := storage.NewMemoryWalletStore()
	audits := storage.NewMemoryAuditStore()
	registry := storage.NewMemoryRegistryStore()
	pricePlans := storage.NewMemoryPricePlanStore()
	seed(tariffs, wallets)
	seedRegistry(registry, tenantID, stationID, userID)
	seedPricePlans(pricePlans, tenantID, stationID)
	slog.Warn("using in-memory stores (no -dsn); admin data is lost on restart")
	return orders, tariffs, wallets, audits, registry, pricePlans
}

func maskDSN(dsn string) string {
	if i := strings.LastIndex(dsn, "@"); i >= 0 && i+1 < len(dsn) {
		return "***@" + dsn[i+1:]
	}
	return dsn
}
