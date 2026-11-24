package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// TDengineStore writes batches through TDengine's REST API. Using REST avoids
// a cgo driver and works with the standard docker image out of the box.
type TDengineStore struct {
	endpoint string // e.g. http://host:6041
	db       string
	client   *http.Client
}

func NewTDengineStore(endpoint, db string) *TDengineStore {
	return &TDengineStore{
		endpoint: strings.TrimRight(endpoint, "/"),
		db:       db,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Init creates the database and stable-table schema if absent.
func (s *TDengineStore) Init(ctx context.Context) error {
	statements := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s KEEP 365", s.db),
		fmt.Sprintf(`CREATE STABLE IF NOT EXISTS %s.meter_values (
			ts TIMESTAMP, voltage_mv BIGINT, current_ma BIGINT, power_w BIGINT,
			energy_wh BIGINT, soc_x100 INT, cable_temp INT
		) TAGS (device_id BINARY(64), connector_id INT)`, s.db),
	}
	for _, sql := range statements {
		if err := s.execSQL(ctx, sql); err != nil {
			return err
		}
	}
	return nil
}

func (s *TDengineStore) execSQL(ctx context.Context, sql string) error {
	body, _ := json.Marshal(map[string]string{"sql": sql})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		s.endpoint+"/api/v2/sql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tdengine sql %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// WriteBatch inserts via an SQL VALUES statement. TDengine REST accepts
// multiple rows in one statement, which is what makes batching effective.
func (s *TDengineStore) WriteBatch(ctxContext any, samples []Sample) error {
	if len(samples) == 0 {
		return nil
	}
	ctx, ok := ctxContext.(context.Context)
	if !ok || ctx == nil {
		ctx = context.Background()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "INSERT INTO %s.meter_values USING %s.meter_values TAGS ('%s', %d) VALUES ",
		s.db, s.db, samples[0].DeviceID, samples[0].ConnectorID)
	for i, sm := range samples {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "(%d, %d, %d, %d, %d, %d, %d)",
			sm.Timestamp.UnixMilli(), sm.VoltageMV, sm.CurrentMA, sm.PowerW,
			sm.EnergyWh, sm.SOCX100, sm.CableTemp)
	}
	return s.execSQL(ctx, b.String())
}

func (s *TDengineStore) Close() error { return nil }
