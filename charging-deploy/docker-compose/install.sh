#!/usr/bin/env bash
set -euo pipefail

echo "[1/4] checking environment"
command -v docker >/dev/null || { echo "docker required"; exit 1; }
[ -f .env ] || { echo "missing .env, copy from .env.example first"; exit 1; }

echo "[2/4] pulling images"
docker compose pull

echo "[3/4] starting services"
docker compose up -d

echo "[4/4] health check"
sleep 5
curl -fsS http://localhost:8080/healthz
curl -fsS http://localhost:9000/healthz
echo "OpenCharge installed."
