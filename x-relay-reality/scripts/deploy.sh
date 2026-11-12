#!/usr/bin/env bash
set -euo pipefail

image="${1:?Usage: bash scripts/deploy.sh ghcr.io/private-gojo/relayx-reality-v2:tag}"
command -v gcloud >/dev/null 2>&1 || { echo "[ERROR] Google Cloud CLI is required." >&2; exit 1; }

SERVICE_NAME="${SERVICE_NAME:-relayx-reality-v2}"
REGION="${REGION:-asia-east1}"
MAX_SESSIONS="${MAX_SESSIONS:-256}"

[[ "$MAX_SESSIONS" =~ ^[0-9]+$ ]] && (( MAX_SESSIONS >= 1 && MAX_SESSIONS <= 1000 )) || {
  echo "[ERROR] MAX_SESSIONS must be from 1 to 1000." >&2
  exit 1
}

echo "[DEPLOY] service=$SERVICE_NAME region=$REGION image=$image"

gcloud run deploy "$SERVICE_NAME" \
  --image "$image" \
  --region "$REGION" \
  --platform managed \
  --allow-unauthenticated \
  --port 8080 \
  --timeout 3600 \
  --concurrency "$MAX_SESSIONS" \
  --set-env-vars "MAX_SESSIONS=$MAX_SESSIONS" \
  --cpu 2 \
  --memory 1Gi \
  --quiet

gcloud run services describe "$SERVICE_NAME" \
  --region "$REGION" \
  --format='value(status.url)'
