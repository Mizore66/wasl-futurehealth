#!/usr/bin/env bash
# Create or update the Wasl read/write API (`wsi7 serve`) on Azure Container Apps (docs/DEPLOY.md, "Azure").
# Needs: az logged in; an existing resource group, ACR holding $IMAGE, Container Apps environment $ENV with an
# Azure Files storage named `waslrun` that holds a finished run directory (cohort/, run/<policy>-cached-<source>/).
#   RG=... LOC=... ACR=... ENV=... IMAGE=<acr>.azurecr.io/wasl:<tag> TOKENS_FILE=<file with WASL_API_TOKENS> \
#     deploy/azure/deploy_api.sh
# Exactly one replica: the review ledger is appended under an in-process mutex plus flock (pipeline/ledger_lock.go).
set -euo pipefail
: "${RG:?}" "${LOC:?}" "${ACR:?}" "${ENV:?}" "${IMAGE:?}" "${TOKENS_FILE:?}"
APP="${APP:-wasl-api}"
UAMI="${UAMI:-wasl-api-pull}"
export MSYS_NO_PATHCONV=1

az identity show -g "$RG" -n "$UAMI" -o none 2>/dev/null || az identity create -g "$RG" -n "$UAMI" -l "$LOC" -o none
UAMI_ID=$(az identity show -g "$RG" -n "$UAMI" --query id -o tsv)
UAMI_PRINCIPAL=$(az identity show -g "$RG" -n "$UAMI" --query principalId -o tsv)
ACR_ID=$(az acr show -n "$ACR" --query id -o tsv)
if [ -z "$(az role assignment list --assignee "$UAMI_PRINCIPAL" --scope "$ACR_ID" --role AcrPull --query '[0].id' -o tsv)" ]; then
  az role assignment create --assignee-object-id "$UAMI_PRINCIPAL" --assignee-principal-type ServicePrincipal \
    --role AcrPull --scope "$ACR_ID" -o none
fi
ENV_ID=$(az containerapp env show -g "$RG" -n "$ENV" --query id -o tsv)

spec=$(mktemp)
trap 'rm -f "$spec"' EXIT
"${PYTHON:-python3}" - "$spec" <<PY
import json, sys
tokens = open("$TOKENS_FILE").read().strip()
env = [{"name": k, "value": v} for k, v in {
    "WASL_OUT": "/var/lib/wasl", "WASL_POLICY": "v7strict", "WASL_SOURCE": "fhir", "WASL_READER": "cached",
    "WASL_SYNTHETIC_DATA_ONLY": "1", "WASL_API_ADDR": "0.0.0.0:8088", "WASL_RUNTIME_ADDR": "127.0.0.1:8090",
    "WASL_RECEIVING_UNIT_CLINICIANS": "${WASL_RECEIVING_UNIT_CLINICIANS:-}",
    "WASL_REFERRING_CLINICIANS": "${WASL_REFERRING_CLINICIANS:-}"}.items() if v]
env.append({"name": "WASL_API_TOKENS", "secretRef": "wasl-api-tokens"})
spec = {
  "location": "$LOC",
  "identity": {"type": "UserAssigned", "userAssignedIdentities": {"$UAMI_ID": {}}},
  "properties": {
    "environmentId": "$ENV_ID",
    "configuration": {
      "activeRevisionsMode": "Single",
      "ingress": {"external": True, "targetPort": 8088, "transport": "http", "allowInsecure": False},
      "registries": [{"server": "$ACR.azurecr.io", "identity": "$UAMI_ID"}],
      "secrets": [{"name": "wasl-api-tokens", "value": tokens}]},
    "template": {
      "containers": [{"name": "api", "image": "$IMAGE", "args": ["serve"], "env": env,
                      "resources": {"cpu": 1.0, "memory": "2Gi"},
                      "volumeMounts": [{"volumeName": "wasl-run", "mountPath": "/var/lib/wasl"}],
                      # Loading the run over SMB takes minutes, not the seconds it takes on a local volume.
                      "probes": [{"type": "Startup", "httpGet": {"path": "/healthz", "port": 8088},
                                  "initialDelaySeconds": 30, "periodSeconds": 30, "failureThreshold": 10},
                                 {"type": "Liveness", "httpGet": {"path": "/healthz", "port": 8088},
                                  "periodSeconds": 30, "failureThreshold": 3},
                                 {"type": "Readiness", "httpGet": {"path": "/healthz", "port": 8088},
                                  "periodSeconds": 10, "failureThreshold": 3}]}],
      "scale": {"minReplicas": 1, "maxReplicas": 1},
      # The ledger append chmods api_reviews.ndjson to 0600; SMB files are owned by root unless the mount makes the
      # API user (Dockerfile UID 10001) the owner, and chmod by a non-owner fails with EPERM.
      "volumes": [{"name": "wasl-run", "storageType": "AzureFile", "storageName": "waslrun",
                   "mountOptions": "uid=10001,gid=10001,file_mode=0600,dir_mode=0700"}]}}}
json.dump(spec, open(sys.argv[1], "w"))
PY

if az containerapp show -g "$RG" -n "$APP" -o none 2>/dev/null; then
  az containerapp update -g "$RG" -n "$APP" --yaml "$spec" -o none
else
  az containerapp create -g "$RG" -n "$APP" --yaml "$spec" -o none
fi
echo "https://$(az containerapp show -g "$RG" -n "$APP" --query properties.configuration.ingress.fqdn -o tsv)"
