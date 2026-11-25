#!/usr/bin/env bash
# OpenCharge local development launcher.
#
#   scripts/dev.sh start              build + start gateway (TCP+admin) and admin UI
#   scripts/dev.sh start --middleware also start Redis/TDengine/Kafka via Docker
#   PostgreSQL and Redis are started by default so admin data and connector
#   state survive restarts and SSE works across gateway instances.
#   scripts/dev.sh stop               stop everything this script started
#   scripts/dev.sh restart            stop + start
#   scripts/dev.sh status             show what is running
#   scripts/dev.sh logs [gateway|frontend|all]
#   scripts/dev.sh seed [count]       run the pile simulator to create N bills
#
# Environment overrides:
#   ADMIN_KEYS     default "key-t1:T1,key-t2:T2"
#   TCP_ADDR       default ":8768"   (pile connections)
#   ADMIN_ADDR     default ":8081"   (admin REST API)
#   FRONTEND_PORT  default "5173"
#   ADMIN_API_KEY  default "key-t1"  (dev proxy injects this into the browser's API calls)
#   PG_DSN         default DSN for the local oc-postgres container
#   NO_POSTGRES=1  disable PostgreSQL and fall back to in-memory stores
#   BCD_ORDER      device-id nibble order, default "low" (real piles)
#   REDIS_ADDR     default 127.0.0.1:16379 (set empty to use memory)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_DIR="$ROOT/.dev"
BIN_DIR="$DEV_DIR/bin"
LOG_DIR="$DEV_DIR/logs"
PID_DIR="$DEV_DIR/pids"

ADMIN_KEYS="${ADMIN_KEYS:-key-t1:T1,key-t2:T2}"
TCP_ADDR="${TCP_ADDR:-:8768}"
ADMIN_ADDR="${ADMIN_ADDR:-:8081}"
FRONTEND_PORT="${FRONTEND_PORT:-5173}"
ADMIN_API_KEY="${ADMIN_API_KEY:-key-t1}"
PG_CONTAINER="${PG_CONTAINER:-oc-postgres}"
PG_DSN="${PG_DSN:-postgres://opencharge:opencharge@127.0.0.1:5432/opencharge?sslmode=disable}"
NO_POSTGRES="${NO_POSTGRES:-0}"
BCD_ORDER="${BCD_ORDER:-low}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:16379}"

# curl must bypass any system HTTP proxy when talking to localhost.
CURL=(curl --noproxy '*' -sS)

mkdir -p "$BIN_DIR" "$LOG_DIR" "$PID_DIR"

host_port() { # ":8081" -> "127.0.0.1:8081"
  printf '127.0.0.1:%s' "${1#:}"
}

is_running() {
  local pidfile="$1"
  [ -f "$pidfile" ] || return 1
  local pid
  pid="$(cat "$pidfile" 2>/dev/null || true)"
  [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

wait_http() {
  local url="$1" name="$2" tries="${3:-40}"
  for ((i=0; i<tries; i++)); do
    if "${CURL[@]}" -o /dev/null --max-time 1 "$url" 2>/dev/null; then
      echo "  [ok] $name ready at $url"
      return 0
    fi
    sleep 0.5
  done
  echo "  [fail] $name did not become ready at $url" >&2
  return 1
}

# Rebuild a binary only when a source file is newer than the existing one.
# The simulator rarely changes, so recompiling it on every start is pure waste.
build_if_stale() {
  local name="$1" pkg="$2" bin="$BIN_DIR/$1"
  local needs_build=0
  if [ ! -x "$bin" ]; then
    needs_build=1
  else
    # Any .go file under the module newer than the binary triggers a rebuild.
    local newer
    newer="$(find "$ROOT/charging-iot-gateway" -name '*.go' -newer "$bin" -print -quit 2>/dev/null || true)"
    [ -n "$newer" ] && needs_build=1
  fi
  if [ "$needs_build" = 1 ]; then
    echo "  [build] $name"
    (cd "$ROOT/charging-iot-gateway" && CGO_ENABLED=0 go build -o "$bin" "$pkg")
  else
    echo "  [skip]  $name (up to date)"
  fi
}

build_all() {
  echo "==> checking binaries"
  build_if_stale gateway ./cmd/gateway
  build_if_stale sim ./device-simulator/ykc
}

# npm's optional-dependency bug (npm/cli#4828) can leave only one darwin
# rollup binding installed. When the system node runs under Rosetta (x64)
# while the managed node is arm64, vite crashes with
# "Cannot find module @rollup/rollup-darwin-*". Install whichever binding is
# missing so either node can start vite; --force bypasses npm's os/cpu guard.
ensure_native_bindings() {
  local admin="$ROOT/charging-frontends/admin"
  local arch pkg ver p
  # Both rollup and esbuild ship native darwin binaries per architecture.
  # Install whichever is missing so vite works under either the managed arm64
  # node or the system x64 node (Rosetta), no matter how it was launched.
  for arch in x64 arm64; do
    for pkg in rollup esbuild; do
      if [ "$pkg" = "rollup" ]; then
        bind_pkg="rollup-darwin-$arch"
      else
        bind_pkg="darwin-$arch"
      fi
      p="$admin/node_modules/@$pkg/$bind_pkg"
      [ -d "$p" ] && continue
      ver="$(NODE_PATH="$admin/node_modules" "$NODE_BIN" -p "require('$pkg/package.json').version" 2>/dev/null || true)"
      [ -z "$ver" ] && continue
      echo "  [fix] installing @$pkg/$bind_pkg@$ver"
      (cd "$admin" && npm install --no-save --force --no-audit --no-fund "@$pkg/$bind_pkg@$ver" >/dev/null 2>&1) || \
        echo "  [warn] could not install $pkg $arch binding"
    done
  done
  # --no-save/--force can still touch manifests on some npm versions.
  (cd "$admin" && git checkout -- package.json package-lock.json 2>/dev/null) || true
}

start_postgres() {
  [ "$NO_POSTGRES" = 1 ] && { echo "  [skip] PostgreSQL disabled (NO_POSTGRES=1, data will not persist)"; return 0; }
  if ! docker info >/dev/null 2>&1; then
    echo "  [warn] docker unavailable; using in-memory stores (data will not persist)" >&2
    return 0
  fi
  if ! docker ps -a --format '{{.Names}}' | grep -qx "$PG_CONTAINER"; then
    echo "  [pg] creating $PG_CONTAINER"
    docker run -d --name "$PG_CONTAINER" \
      -e POSTGRES_DB=opencharge -e POSTGRES_USER=opencharge -e POSTGRES_PASSWORD=opencharge \
      -p 5432:5432 postgres:16-alpine >/dev/null 2>&1 || {
        echo "  [warn] could not start PostgreSQL; falling back to memory" >&2
        return 0
      }
  fi
  docker start "$PG_CONTAINER" >/dev/null 2>&1 || true
  # Wait for the database to actually accept connections before the gateway
  # tries to migrate; a sleep alone is not enough on first boot.
  for _ in $(seq 1 40); do
    if docker exec "$PG_CONTAINER" pg_isready -U opencharge -d opencharge >/dev/null 2>&1; then
      echo "  [ok] PostgreSQL ready at 127.0.0.1:5432"
      return 0
    fi
    sleep 0.5
  done
  echo "  [warn] PostgreSQL not ready; falling back to in-memory stores" >&2
}

start_redis() {
  [ -z "$REDIS_ADDR" ] && { echo "  [skip] Redis disabled (state kept in memory)"; return 0; }
  if ! docker info >/dev/null 2>&1; then
    echo "  [warn] docker unavailable; connector state will be in memory" >&2
    return 0
  fi
  if ! docker ps -a --format '{{.Names}}' | grep -qx oc-redis; then
    echo "  [redis] creating oc-redis"
    docker run -d --name oc-redis -p 16379:6379 redis:7-alpine --appendonly yes >/dev/null 2>&1 || {
      echo "  [warn] could not start Redis" >&2
      return 0
    }
  fi
  docker start oc-redis >/dev/null 2>&1 || true
  for _ in $(seq 1 20); do
    if docker exec oc-redis redis-cli ping 2>/dev/null | grep -q PONG; then
      echo "  [ok] Redis ready at $REDIS_ADDR"
      return 0
    fi
    sleep 0.25
  done
  echo "  [warn] Redis not ready; state will be in memory" >&2
}

start_middleware() {
  echo "==> starting optional middleware (Redis :16379, TDengine :16041, Kafka :19092)"
  # Redis is light and verified; TDengine/Kafka can be memory-limited on some
  # hosts, so failures here are warnings rather than fatal.
  if ! docker info >/dev/null 2>&1; then
    echo "  [warn] docker daemon unavailable; skipping middleware (memory fallback will be used)"
    return 0
  fi

  if ! docker ps -a --format '{{.Names}}' | grep -qx 'oc-tdengine'; then
    docker run -d --name oc-tdengine -p 16041:6041 tdengine/tdengine:3.2.3.0 >/dev/null 2>&1 || true
  fi
  docker start oc-tdengine >/dev/null 2>&1 || true

  if ! docker ps -a --format '{{.Names}}' | grep -qx 'oc-kafka'; then
    docker run -d --name oc-kafka -p 19092:9092 \
      -e KAFKA_NODE_ID=1 -e KAFKA_PROCESS_ROLES=broker,controller \
      -e KAFKA_LISTENERS=PLAINTEXT://:9092,CONTROLLER://:9093 \
      -e KAFKA_ADVERTISED_LISTENERS=PLAINTEXT://127.0.0.1:19092 \
      -e KAFKA_CONTROLLER_QUORUM_VOTERS=1@127.0.0.1:9093 \
      -e KAFKA_CONTROLLER_LISTENER_NAMES=CONTROLLER \
      -e KAFKA_LISTENER_SECURITY_PROTOCOL_MAP=CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT \
      -e KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1 \
      -e KAFKA_AUTO_CREATE_TOPICS_ENABLE=true apache/kafka:3.7.0 >/dev/null 2>&1 || true
  fi
  docker start oc-kafka >/dev/null 2>&1 || true

  echo "  middleware containers requested; use 'docker ps --filter name=oc-' to confirm which survived"
}

resolve_node() {
  # On Apple Silicon the system node may run under Rosetta as x64 while npm
  # installed the arm64 native rollup binding. Pick a node matching node_modules
  # so vite does not die with "Cannot find module @rollup/rollup-darwin-*".
  local host_arch
  host_arch="$(uname -m)"
  for candidate in /Users/joe/.workbuddy/binaries/node/versions/22.22.2/bin/node "$(command -v node)"; do
    [ -n "$candidate" ] && [ -x "$candidate" ] || continue
    local arch
    arch="$("$candidate" -p 'process.arch' 2>/dev/null || true)"
    case "$host_arch:$arch" in
      arm64:arm64|x86_64:x64)
        NODE_BIN="$candidate"
        echo "  [ok] using node $candidate ($arch)"
        return 0
        ;;
    esac
  done
  NODE_BIN="$(command -v node)"
  echo "  [warn] no arch-matched node found; vite may fail to start"
}

start_all() {
  local with_middleware=0
  [ "${1:-}" = "--middleware" ] && with_middleware=1

  if is_running "$PID_DIR/gateway.pid" || is_running "$PID_DIR/frontend.pid"; then
    echo "already running. use '$0 status' or '$0 restart'." >&2
    exit 1
  fi

  start_postgres
  start_redis
  build_all
  [ "$with_middleware" = 1 ] && start_middleware

  echo "==> starting gateway (piles $TCP_ADDR, admin $ADMIN_ADDR)"
  # -redis/-tdengine/-kafka are appended only when middleware was requested;
  # otherwise the gateway uses its verified in-memory defaults.
  local extra=()
  if [ "$with_middleware" = 1 ]; then
    extra=(-redis 127.0.0.1:16379 -tdengine http://127.0.0.1:16041 -kafka 127.0.0.1:19092)
  fi
  # Bash 3.2 (macOS default) treats an empty array under `set -u` as unbound;
  # the +"${...}" form expands to nothing when the array has no entries.
  local pg_args=()
  [ "$NO_POSTGRES" != 1 ] && docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$PG_CONTAINER" && \
    pg_args=(-dsn "$PG_DSN")
  nohup "$BIN_DIR/gateway" \
    -addr "$TCP_ADDR" -admin "$ADMIN_ADDR" -admin-key "$ADMIN_KEYS" \
    -bcd-order "$BCD_ORDER" -redis "$REDIS_ADDR" ${pg_args[@]+"${pg_args[@]}"} ${extra[@]+"${extra[@]}"} >"$LOG_DIR/gateway.log" 2>&1 &
  echo $! >"$PID_DIR/gateway.pid"

  echo "==> starting admin UI (:${FRONTEND_PORT})"
  resolve_node
  ensure_native_bindings
  (
    cd "$ROOT/charging-frontends/admin"
    ADMIN_API_KEY="$ADMIN_API_KEY" \
    local vite_js="$ROOT/charging-frontends/admin/node_modules/vite/bin/vite.js"
    [ -f "$vite_js" ] || { echo "  [fail] vite not installed; run: cd charging-frontends/admin && npm install" >&2; exit 1; }
    nohup "$NODE_BIN" "$vite_js" --port "$FRONTEND_PORT" --strictPort \
      >"$LOG_DIR/frontend.log" 2>&1 &
    echo $! >"$PID_DIR/frontend.pid"
  )

  wait_http "http://127.0.0.1:${ADMIN_ADDR#:}/healthz" "admin API" || true
  wait_http "http://127.0.0.1:${FRONTEND_PORT}/" "admin UI" || true

  echo
  echo "OpenCharge is up:"
  echo "  admin UI      http://localhost:${FRONTEND_PORT}"
  echo "  admin API     http://localhost:${ADMIN_ADDR#:}   (X-API-Key: ${ADMIN_API_KEY%%:*})"
  echo "  pile TCP      localhost:${TCP_ADDR#:}"
  echo "  logs          $LOG_DIR"
  echo "  seed a bill   $0 seed"
}

stop_all() {
  echo "==> stopping services"
  local name pid
  for name in frontend gateway; do
    local pf="$PID_DIR/$name.pid"
    if [ -f "$pf" ]; then
      pid="$(cat "$pf")"
      if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
        for _ in 1 2 3 4 5 6 7 8 9 10; do
          kill -0 "$pid" 2>/dev/null || break
          sleep 0.2
        done
        kill -9 "$pid" 2>/dev/null || true
        echo "  [ok] stopped $name (pid $pid)"
      fi
      rm -f "$pf"
    fi
  done

  # Vite can leave child node processes; clean up anything still bound to the
  # configured port that is not one of our tracked pids.
  local leftover
  leftover="$(lsof -ti "tcp:${FRONTEND_PORT}" 2>/dev/null || true)"
  if [ -n "$leftover" ]; then
    echo "$leftover" | xargs kill -9 2>/dev/null || true
  fi
}

show_status() {
  echo "==> service status"
  local name pf pid
  for name in gateway frontend; do
    pf="$PID_DIR/$name.pid"
    if is_running "$pf"; then
      pid="$(cat "$pf")"
      printf "  %-10s RUNNING (pid %s)\n" "$name" "$pid"
    else
      printf "  %-10s stopped\n" "$name"
    fi
  done
  if [ "$NO_POSTGRES" = 1 ]; then
    printf "  %-10s disabled\n" "postgres"
  elif docker ps --format '{{.Names}}' 2>/dev/null | grep -qx "$PG_CONTAINER"; then
    printf "  %-10s RUNNING (data persisted in %s)\n" "postgres" "$PG_CONTAINER"
  else
    printf "  %-10s stopped (gateway falls back to memory)\n" "postgres"
  fi
  echo
  echo "  ports:"
  for p in "${TCP_ADDR#:}" "${ADMIN_ADDR#:}" "${FRONTEND_PORT}"; do
    local owner
    owner="$(lsof -nP -iTCP:"$p" -sTCP:LISTEN 2>/dev/null | tail -1 | awk '{print $1, $2}')"
    printf "    %-6s %s\n" "$p" "${owner:-free}"
  done
}

show_logs() {
  local which="${1:-all}"
  case "$which" in
    gateway) tail -n 80 "$LOG_DIR/gateway.log" 2>/dev/null || true ;;
    frontend) tail -n 80 "$LOG_DIR/frontend.log" 2>/dev/null || true ;;
    all)
      echo "--- gateway ---"; tail -n 40 "$LOG_DIR/gateway.log" 2>/dev/null || true
      echo "--- frontend ---"; tail -n 40 "$LOG_DIR/frontend.log" 2>/dev/null || true
      ;;
    *) echo "usage: $0 logs [gateway|frontend|all]" >&2; exit 1 ;;
  esac
}

seed_bills() {
  local count="${1:-1}"
  is_running "$PID_DIR/gateway.pid" || { echo "gateway is not running; run '$0 start' first" >&2; exit 1; }
  echo "==> generating $count simulated bill(s)"
  # These device ids are pre-registered by the gateway's seedRegistry(), so
  # each bill resolves a tenant/station from master data instead of falling back.
  local devices=(55031412782305 55031412782306 55031412782307)
  for ((i=0; i<count; i++)); do
    local dev="${devices[$((i % ${#devices[@]}))]}"
    "$BIN_DIR/sim" -addr "127.0.0.1:${TCP_ADDR#:}" \
      -device "$dev" -gun 1 \
      >"$LOG_DIR/seed-$i.log" 2>&1 || { echo "  [fail] see $LOG_DIR/seed-$i.log"; continue; }
    echo "  [ok] bill $i from $dev"
    sleep 0.3
  done
  "${CURL[@]}" -H "X-API-Key: ${ADMIN_API_KEY}" \
    "http://127.0.0.1:${ADMIN_ADDR#:}/api/admin/orders" | head -c 300
  echo
}

case "${1:-}" in
  start) shift; start_all "$@" ;;
  stop) stop_all ;;
  restart) stop_all; start_all "${2:-}" ;;
  status) show_status ;;
  logs) shift; show_logs "${1:-all}" ;;
  seed) shift; seed_bills "${1:-1}" ;;
  *)
    sed -n '2,16p' "$0" | sed 's/^# \{0,1\}//'
    exit 1
    ;;
esac
