-- OpenCharge initial schema (PostgreSQL)
CREATE TABLE IF NOT EXISTS tenant (
  id           VARCHAR(64) PRIMARY KEY,
  name         VARCHAR(128) NOT NULL,
  parent_id    VARCHAR(64),
  status       VARCHAR(16) NOT NULL DEFAULT 'active',
  created_at   BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS station (
  id           VARCHAR(64) PRIMARY KEY,
  tenant_id    VARCHAR(64) NOT NULL,
  name         VARCHAR(128) NOT NULL,
  timezone     VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
  created_at   BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_station_tenant ON station(tenant_id);

CREATE TABLE IF NOT EXISTS device (
  device_id    VARCHAR(64) PRIMARY KEY,
  tenant_id    VARCHAR(64) NOT NULL,
  station_id   VARCHAR(64) NOT NULL,
  protocol     VARCHAR(32) NOT NULL,
  secret       VARCHAR(256) NOT NULL,
  last_seen_at BIGINT
);

CREATE TABLE IF NOT EXISTS connector (
  device_id    VARCHAR(64) NOT NULL,
  gun_no       INT NOT NULL,
  tenant_id    VARCHAR(64) NOT NULL,
  station_id   VARCHAR(64) NOT NULL,
  status       VARCHAR(32) NOT NULL,
  PRIMARY KEY (device_id, gun_no)
);

-- All monetary columns use DECIMAL(18,4); never FLOAT/DOUBLE.
CREATE TABLE IF NOT EXISTS tariff_version (
  id           VARCHAR(64) PRIMARY KEY,
  tenant_id    VARCHAR(64) NOT NULL,
  station_id   VARCHAR(64) NOT NULL,
  effective_at BIGINT NOT NULL,
  slots_json   JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS charging_order (
  id                VARCHAR(64) PRIMARY KEY,
  tenant_id         VARCHAR(64) NOT NULL,
  station_id        VARCHAR(64) NOT NULL,
  device_id         VARCHAR(64) NOT NULL,
  gun_no            INT NOT NULL,
  transaction_id    VARCHAR(64) NOT NULL UNIQUE,
  state             VARCHAR(32) NOT NULL,
  tariff_version_id VARCHAR(64) NOT NULL,
  energy_wh         BIGINT NOT NULL DEFAULT 0,
  fee               DECIMAL(18,4) NOT NULL DEFAULT 0,
  offline           BOOLEAN NOT NULL DEFAULT FALSE,
  started_at        BIGINT,
  stopped_at        BIGINT
);
CREATE INDEX IF NOT EXISTS idx_order_tenant ON charging_order(tenant_id);
CREATE INDEX IF NOT EXISTS idx_order_tx ON charging_order(transaction_id);

CREATE TABLE IF NOT EXISTS wallet (
  user_id     VARCHAR(64) NOT NULL,
  tenant_id   VARCHAR(64) NOT NULL,
  principal   DECIMAL(18,4) NOT NULL DEFAULT 0,
  bonus       DECIMAL(18,4) NOT NULL DEFAULT 0,
  frozen      DECIMAL(18,4) NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, tenant_id)
);

CREATE TABLE IF NOT EXISTS tenant_grant (
  id         VARCHAR(64) PRIMARY KEY,
  user_id    VARCHAR(64) NOT NULL,
  tenant_id  VARCHAR(64) NOT NULL,
  scopes_json JSONB,
  created_at BIGINT NOT NULL,
  revoked_at BIGINT
);
CREATE INDEX IF NOT EXISTS idx_grant_user ON tenant_grant(user_id);

-- Billing audit and exception tracking
CREATE TABLE IF NOT EXISTS order_segments (
  id           BIGSERIAL PRIMARY KEY,
  order_id     VARCHAR(64) NOT NULL,
  period       VARCHAR(16) NOT NULL,
  price        DECIMAL(18,5) NOT NULL,
  energy_units BIGINT NOT NULL,
  amount       DECIMAL(18,4) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_segments_order ON order_segments(order_id);

CREATE TABLE IF NOT EXISTS billing_audit (
  id              VARCHAR(64) PRIMARY KEY,
  transaction_id  VARCHAR(64) NOT NULL,
  rule            VARCHAR(32) NOT NULL,
  pile_amount     DECIMAL(18,4) NOT NULL,
  platform_amount DECIMAL(18,4) NOT NULL,
  diff            DECIMAL(18,4) NOT NULL,
  charge_amount   DECIMAL(18,4) NOT NULL,
  trace_id        VARCHAR(64),
  created_at      BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_txn ON billing_audit(transaction_id);

CREATE TABLE IF NOT EXISTS exception_order (
  id              VARCHAR(64) PRIMARY KEY,
  transaction_id  VARCHAR(64) NOT NULL,
  reason_code     VARCHAR(64) NOT NULL,
  pile_amount     DECIMAL(18,4) NOT NULL,
  platform_amount DECIMAL(18,4) NOT NULL,
  diff            DECIMAL(18,4) NOT NULL,
  trace_id        VARCHAR(64),
  created_at      BIGINT NOT NULL,
  resolved        BOOLEAN NOT NULL DEFAULT FALSE,
  resolved_by     VARCHAR(64)
);
CREATE INDEX IF NOT EXISTS idx_exception_txn ON exception_order(transaction_id);

CREATE TABLE IF NOT EXISTS applied_debit (
  txn_id VARCHAR(64) PRIMARY KEY
);

-- Add settlement columns to charging_order if upgrading an existing schema
ALTER TABLE charging_order ADD COLUMN IF NOT EXISTS pile_amount DECIMAL(18,4) NOT NULL DEFAULT 0;
ALTER TABLE charging_order ADD COLUMN IF NOT EXISTS platform_amount DECIMAL(18,4) NOT NULL DEFAULT 0;
ALTER TABLE charging_order ADD COLUMN IF NOT EXISTS charge_amount DECIMAL(18,4) NOT NULL DEFAULT 0;
ALTER TABLE charging_order ADD COLUMN IF NOT EXISTS exception_reason VARCHAR(64);
ALTER TABLE charging_order ADD COLUMN IF NOT EXISTS station_id VARCHAR(64);

-- Adjustable pricing plans (admin 调整定价). Separate from tariff_version
-- which is the 48-slot settlement snapshot; this table holds editable
-- time windows with electricity/service fee split for display and downlink.
CREATE TABLE IF NOT EXISTS price_plan (
  id              VARCHAR(64) PRIMARY KEY,
  tenant_id       VARCHAR(64) NOT NULL,
  station_id      VARCHAR(64) NOT NULL,
  device_type     VARCHAR(128) NOT NULL,
  effective_at    BIGINT NOT NULL,
  effective_now   BOOLEAN NOT NULL DEFAULT TRUE,
  status          VARCHAR(16) NOT NULL,
  source          VARCHAR(32) NOT NULL,
  operator        VARCHAR(64),
  note            VARCHAR(256),
  delivered_count INT NOT NULL DEFAULT 0,
  success_count   INT NOT NULL DEFAULT 0,
  created_at      BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_price_plan_scope ON price_plan(tenant_id, station_id, device_type, status);

CREATE TABLE IF NOT EXISTS price_plan_segment (
  id            SERIAL PRIMARY KEY,
  plan_id       VARCHAR(64) NOT NULL,
  seq           INT NOT NULL,
  start_min     INT NOT NULL,
  end_min       INT NOT NULL,
  period        VARCHAR(16) NOT NULL,
  elec_price    BIGINT NOT NULL,
  service_price BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_price_plan_segment_plan ON price_plan_segment(plan_id);
