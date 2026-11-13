#!/usr/bin/env bash
set -euo pipefail

# Deploy x-relay-reality to Google Cloud Run
PROJECT_ID="${PROJECT_ID:-$(gcloud config get-value project)}"
REGION="${REGION:-asia-east1}"
SERVICE_NAME="${SERVICE_NAME:-relayx-reality-v2}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="$(dirname "${SCRIPT_DIR}")"

echo "Deploying ${SERVICE_NAME} from ${APP_DIR} to Cloud Run (Project: ${PROJECT_ID}, Region: ${REGION})..."

gcloud run deploy "${SERVICE_NAME}" \
  --source="${APP_DIR}" \
  --region="${REGION}" \
  --platform=managed \
  --allow-unauthenticated \
  --port=8080 \
  --timeout=3600 \
  --memory=512Mi \
  --cpu=1 \
  --set-env-vars="MODE=server,WS_PATH=/reality,CHAIN_PATH=/_chain,XHTTP_PATH=/xh,REALITY_DEST=firebaseremoteconfigrealtime.googleapis.com:443,REALITY_SERVER_NAMES=googleapis.com^:^firebaseremoteconfigrealtime.googleapis.com" \
  --quiet

SERVICE_URL="$(gcloud run services describe "${SERVICE_NAME}" --region="${REGION}" --format='value(status.url)')"
SERVICE_HOST="${SERVICE_URL#https://}"

echo "=========================================="
echo " Deployment Complete!"
echo " Service URL : ${SERVICE_URL}"
echo " Service Host: ${SERVICE_HOST}"
echo " NetMod JSON : ${SERVICE_URL}/profile.json"
echo "=========================================="
