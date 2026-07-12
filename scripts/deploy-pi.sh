#!/usr/bin/env bash
set -euo pipefail

cd /home/mikhail/projects/hermes-dashboard

echo "== Hermes deploy startet =="

echo "== Git status =="
git status --short

echo "== Updates ziehen =="
git pull --ff-only

echo "== Backup-Ordner vorbereiten =="
mkdir -p "$HOME/backups/hermes"

echo "== Datenbank-Backup erstellen =="
docker compose exec -T db pg_dump -U hermes -d hermes > "$HOME/backups/hermes/hermes-backup-$(date +%F-%H%M).sql"

echo "== Docker neu bauen und starten =="
docker compose up -d --build

echo "== Containerstatus =="
docker compose ps

echo "== Hermes deploy fertig =="