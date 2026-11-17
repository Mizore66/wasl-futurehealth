# Wasl on Azure: API on Container Apps, audit agent on Foundry

Status and evidence are in [`docs/DEPLOY.md`, "Azure"](../../docs/DEPLOY.md#azure). This file is the runbook.
All data is synthetic. Keep `WASL_SYNTHETIC_DATA_ONLY=1`.

| Part | Where | What it is |
|---|---|---|
| `wsi7 serve` API | Azure Container Apps, one replica | The read/write API over a finished run. The run directory lives on an Azure Files share. |
| Pipeline run | Local Docker (`docker compose`, 3-node NATS) | Produces the run directory the API serves. It is **not** run on Azure. |
| `wasl-audit-explainer` | Foundry hosted agent (`agent/`) | Reads one case's redacted audit trail with an auditor token and explains it with `gpt-4.1-mini`. Read-only. |
| `gpt-4.1-mini` | Foundry model deployment, Global Standard | The agent's model. It is not the Gemma reader; the published results do not use it. |

## Prerequisites

- `az` logged in, and `azd` 1.36 or later with `azd extension install microsoft.foundry`.
  Run `azd config set auth.useAzCliAuth true` to reuse the `az` login.
- A region that offers both hosted agents and the model. This deployment uses `japaneast`.
- On Windows Git Bash, set `MSYS_NO_PATHCONV=1` before any command that passes an Azure resource ID.
  Otherwise Git Bash rewrites `/subscriptions/...` into a Windows path.
- The checkout must have LF line endings for `*.sh`. `.gitattributes` enforces this. A CRLF checkout builds images
  whose entrypoints fail with `no such file or directory`.

## 1. Produce the run directory locally

```bash
export COMPOSE_PROJECT_NAME=waslazure WASL_JS_KEY="k$(openssl rand -hex 32)"
docker compose build nats-0 pipeline
docker compose up --abort-on-container-failure --exit-code-from pipeline nats-0 nats-1 nats-2 pipeline
MSYS_NO_PATHCONV=1 docker run --rm -v waslazure_wasl-run:/v:ro alpine tar czf - -C /v . > wasl-run.tgz
```

Build `nats-0` once before `up`. The three NATS services share one image tag, and building them in parallel fails
with `image "docker.io/library/wasl-nats:local": already exists`.

## 2. Azure resources

```bash
RG=rg-wasl-demo LOC=japaneast
az group create -n $RG -l $LOC
az acr create -g $RG -n <acr> --sku Basic
az acr login -n <acr> && docker tag wasl:local <acr>.azurecr.io/wasl:<tag> && docker push <acr>.azurecr.io/wasl:<tag>
az storage account create -g $RG -n <sa> --sku Standard_LRS --min-tls-version TLS1_2 --allow-blob-public-access false
az storage share-rm create -g $RG --storage-account <sa> -n wasl-run --quota 5
mkdir run && tar xzf wasl-run.tgz -C run
az storage file upload-batch --account-name <sa> --account-key <key> -d wasl-run -s run
```

The Container Apps environment needs its own VNet. Environments created without one came out as "express"
environments, which refuse Azure Files volumes (`ExpressVolumeStorageTypeNotSupported`).

```bash
az network vnet create -g $RG -n wasl-vnet --address-prefixes 10.40.0.0/16 --subnet-name aca --subnet-prefixes 10.40.0.0/23
az network vnet subnet update -g $RG --vnet-name wasl-vnet -n aca --delegations Microsoft.App/environments
az containerapp env create -g $RG -n wasl-env-vnet -l $LOC --enable-workload-profiles true \
  --infrastructure-subnet-resource-id "$(az network vnet subnet show -g $RG --vnet-name wasl-vnet -n aca --query id -o tsv)"
az containerapp env storage set -g $RG -n wasl-env-vnet --storage-name waslrun --azure-file-account-name <sa> \
  --azure-file-account-key <key> --azure-file-share-name wasl-run --access-mode ReadWrite
```

## 3. The API

Generate fresh tokens as in [`docs/DEPLOY.md`, "Hosted frontend"](../../docs/DEPLOY.md#hosted-frontend-vercel).
Never use the published demo tokens. Put the `WASL_API_TOKENS` value in a file outside the repository.

```bash
RG=... LOC=... ACR=<acr> ENV=wasl-env-vnet IMAGE=<acr>.azurecr.io/wasl:<tag> TOKENS_FILE=<file> \
  deploy/azure/deploy_api.sh
```

`deploy_api.sh` creates or updates the app. It runs one replica, keeps the tokens as a Container Apps secret, and pulls
the image with a user-assigned identity. It also mounts the share with `uid=10001,gid=10001,file_mode=0600,dir_mode=0700`.
The ledger append chmods `api_reviews.ndjson` to 0600, and on a default mount the files belong to root, so that chmod
fails with EPERM and every write returns 500. The startup probe allows about five minutes, because loading the run
over SMB is slower than from a local volume.

Verify, restart, verify again:

```bash
python3 deploy/azure/verify_api.py write  https://<fqdn> <dir with wasl-reviewer.token and wasl-coordinator.token>
az containerapp revision restart -g $RG -n wasl-api --revision <active revision>
# wait until only the replica created after the write is listed (az containerapp replica list)
python3 deploy/azure/verify_api.py reread https://<fqdn> <same dir>
```

## 4. The model and the agent

```bash
az cognitiveservices account create -g $RG -n <ai> -l $LOC --kind AIServices --sku S0 --custom-domain <ai> --allow-project-management
az cognitiveservices account project create -g $RG -n <ai> --project-name wasl-agents -l $LOC
az cognitiveservices account deployment create -g $RG -n <ai> --deployment-name gpt-4.1-mini \
  --model-name gpt-4.1-mini --model-version 2025-04-14 --model-format OpenAI --sku-name GlobalStandard --sku-capacity 20
```

Put the auditor token in Key Vault (RBAC mode) as `wasl-auditor-token`. Then, from `deploy/azure/`:

```bash
azd env set WASL_API_URL https://<fqdn>
azd env set WASL_KEYVAULT_URL https://<kv>.vault.azure.net/
azd deploy wasl-audit-explainer
azd ai agent show wasl-audit-explainer        # instance_identity.principal_id
```

Grant that principal `Key Vault Secrets User` on the vault. The working deployment also granted it both
`Foundry User` and `Cognitive Services OpenAI User` on the Foundry account. Which of the two the model call needs is
untested. If a new agent version still fails with `AuthenticationError` after the grants, redeploy it. Then:

```bash
azd ai agent invoke wasl-audit-explainer "Explain the audit trail for ahs-mushrif/ServiceRequest/AHS-sr-1001"
```

The agent makes one GET to `/api/cases/{id}/audit` and one model call. It sends nothing to any write route.

## Cost and teardown

The API runs one always-on replica (1 vCPU, 2 GiB). That is the largest cost: roughly USD 1.5 to 2 a day at list
consumption prices, which is an estimate, not a measured bill. ACR Basic, the storage account and the Key Vault cost
cents a day. The model and the agent bill per use. Delete everything with `az group delete -n rg-wasl-demo`.
