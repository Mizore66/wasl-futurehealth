# Wasl hosted demo: API on Azure, audit agent on Foundry, Gemma 3 4B on Modal

Status and evidence are in [`docs/DEPLOY.md`, "Azure"](../../docs/DEPLOY.md#azure). This file is the runbook.
All data is synthetic. Keep `WASL_SYNTHETIC_DATA_ONLY=1`.

Wasl's language model is the small open model the repository's reader already uses: Gemma 3 4B as
`gemma-3-4b-it-Q4_K_M.gguf`, served by `llama-server`. That file runs on a hospital's own laptop or server, so patient
text never has to reach a cloud-hosted model. For this hosted demo the same file runs on a rented GPU on Modal, which
stands in for the hospital's machine and only ever sees synthetic data. No frontier model is in the request path.

| Part | Where | What it is |
|---|---|---|
| `wsi7 serve` API | Azure Container Apps, one replica | The read/write API over a finished run. The run directory lives on an Azure Files share. |
| Pipeline run | Local Docker (`docker compose`, 3-node NATS) | Produces the run directory the API serves. It is **not** run on Azure. |
| `wasl-audit-explainer` | Foundry hosted agent (`agent/`) | Reads one case's redacted audit trail with an auditor token and explains it with Gemma 3 4B. Read-only. |
| Gemma 3 4B (`gemma-3-4b-it-Q4_K_M.gguf`) | Modal, one T4 GPU, scales to zero (`deploy/modal/gemma_server.py`) | The on-device reader's exact file and `llama-server` flags, behind an API key. |
| `gpt-4.1-mini` | Foundry model deployment, unused | Created during the first Azure deploy. Nothing calls it; delete it with `az cognitiveservices account deployment delete`. |

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

## 4. Gemma 3 4B on Modal

Modal needs a payment method on the account before it runs a GPU; the Starter plan's monthly credit still applies.
Generate a random server key, then:

```bash
python -m modal token new                                       # browser sign-in
python -m modal secret create wasl-gemma-key --from-dotenv <file with LLAMA_API_KEY=...>
az keyvault secret set --vault-name <kv> -n gemma-api-key --file <file with the key>
python -m modal run    deploy/modal/gemma_server.py::download   # fetch ggml-org's file, check its SHA-256
python -m modal deploy deploy/modal/gemma_server.py             # WASL_GEMMA_WARM=1 keeps one GPU up
```

The server refuses every model route without the key (401); `/health` stays open. It scales to zero after 15 idle
minutes. Measured on 18 Nov 2026, one sample each: a cold request after a fresh deploy took 135 s and returned 200,
including the image pull; a warm request took 1.3 s; generation ran at 50.6 tokens per second. During judging, deploy
with `WASL_GEMMA_WARM=1` so no visitor waits for a cold start, then redeploy without it.

## 5. The agent

```bash
az cognitiveservices account create -g $RG -n <ai> -l $LOC --kind AIServices --sku S0 --custom-domain <ai> --allow-project-management
az cognitiveservices account project create -g $RG -n <ai> --project-name wasl-agents -l $LOC
```

Put the auditor token in Key Vault (RBAC mode) as `wasl-auditor-token`. Then, from `deploy/azure/`:

```bash
azd env set WASL_API_URL https://<fqdn>
azd env set WASL_KEYVAULT_URL https://<kv>.vault.azure.net/
azd env set WASL_LLM_URL https://<workspace>--wasl-gemma-serve.us-east.modal.run
azd env set WASL_LLM_MODEL gemma-3-4b-it-Q4_K_M
azd deploy wasl-audit-explainer
azd ai agent show wasl-audit-explainer        # instance_identity.principal_id
```

Grant that principal `Key Vault Secrets User` on the vault: it reads `wasl-auditor-token` and `gemma-api-key` there.
Then:

```bash
azd ai agent invoke wasl-audit-explainer "Explain the audit trail for ahs-mushrif/ServiceRequest/AHS-sr-1001"
```

The agent makes one GET to `/api/cases/{id}/audit` and one call to the Gemma server. It sends nothing to any write
route.

## 6. The web app on Vercel

The Vercel project `wasl` has Root Directory `web`. Deploy from the repository root: the root `.vercelignore`
uploads only `web/` and `data/evidence/`, which `next build` type-checks through
`web/tests/review-source-fixture.test.ts`. A deploy from inside `web/` fails on that missing file.

Production variables (server-only, never `NEXT_PUBLIC_`): `WASL_API_URL` and the tokens the server reads:
`WASL_API_WEB_REVIEWER_TOKEN` (actor `hosted-web-reviewer`, not an assigned reader), `WASL_API_AUDITOR_TOKEN`,
`WASL_API_COORDINATOR_TOKEN`, `WASL_API_CLINICIAN_TOKEN` (the unit clinician in `WASL_RECEIVING_UNIT_CLINICIANS`) and
`WASL_API_REFERRER_TOKEN` (the referrer in `WASL_REFERRING_CLINICIANS`). Personal reviewer and clinician tokens are not
set there; people type them into the sign-in form. The API must be deployed with
`WASL_RECEIVING_UNIT_CLINICIANS=skmc-endo:hosted-unit-clinician WASL_REFERRING_CLINICIANS=ahs-mushrif:hosted-referrer`.

The production alias is public by decision (18 Nov 2026), so that judges can open it without a Vercel account.
Anyone who opens it acts as the coordinator, unit clinician and referrer actors on the synthetic ledger. Deployment
URLs other than the production alias stay behind Vercel Authentication. To close the alias, turn on Vercel protection
for it.

## Continuous deployment

`.github/workflows/deploy.yml` runs on every push to `main` and on manual dispatch. CI runs only on pull requests,
and branch protection admits a merge only after "CI required" passes, so `main` is already tested when Deploy starts.
Each job runs only when its inputs changed:

| Job | Triggered by changes to | Does | Checks |
|---|---|---|---|
| API | `pipeline/`, `scripts/`, `data/`, `models/`, `booking/`, `patient-parsers/`, `Dockerfile`, the API deploy files | builds the image on the runner, pushes `wasl:<sha>` to ACR, rolls the Container App onto it | the new revision becomes ready and runs the new image (a failed revision leaves the old one serving, which a smoke test alone would miss), then `verify_api.py smoke` |
| Agent | `deploy/azure/agent/`, `azure.yaml` | runs the agent tests, rebuilds the `azd` environment from `ci.env`, `azd deploy` | the deployed agent answers a no-case-id prompt |
| Web | `web/`, `data/evidence/`, `.vercelignore` | `vercel pull`, `vercel build --prod`, `vercel deploy --prebuilt --prod` | the deployment reports Ready |
| Model | `deploy/modal/` | `modal deploy` of the Gemma server | `/health` reaches 200 and an unauthenticated model route gets 401 |

GitHub signs in to Azure as the user-assigned identity `wasl-github-deploy`, through federated credentials that trust
only `main`. GitHub sends this repository's subject with immutable owner and repository IDs,
`repo:Mizore66@123628501/wasl-futurehealth@1431169101:ref:refs/heads/main`, not the plain `repo:<owner>/<repo>` form;
a credential for the plain form alone fails with `AADSTS700213`. Both forms are configured. No Azure secret exists. The identity holds
Contributor on the resource group, AcrPush on the registry, and Foundry Project Manager and Foundry User on the
Foundry account. App registrations are blocked in this tenant, which is why it is a managed identity.

Setup, done once and repeated when the tokens expire: run `bash deploy/azure/ci_setup.sh` on a machine with `az` and
`vercel link` set up, from a GitHub account allowed to set the repository's Actions variables and secrets (write
access sufficed on 18 Nov 2026; admin was not needed). It sets five repository variables: the identity's client ID,
the tenant, the subscription, and the Vercel org and project IDs. It then runs `ci_secrets.sh`, which creates a 30-day
Modal token after you approve it in the browser and asks, at a hidden prompt, for a Vercel token you create for the
`kais-projects-42abda1c` team. Each token goes straight into a GitHub secret (`MODAL_TOKEN_ID`, `MODAL_TOKEN_SECRET`,
`VERCEL_TOKEN`) and is never printed. The Modal and Vercel tokens expire 30 days after creation; rerun the script then.

Not automated: the pipeline run that produces the API's data. A new run invalidates the review ledger (`docs/API.md`),
so refreshing the share stays a deliberate, manual step (section 1 and 2).

## Cost and teardown

The API runs one always-on replica (1 vCPU, 2 GiB). That is the largest cost: roughly USD 1.5 to 2 a day at list
consumption prices, which is an estimate, not a measured bill. ACR Basic, the storage account and the Key Vault cost
cents a day. The Foundry agent bills per use. Modal bills the T4 per second while a container runs, about USD 0.59 an
hour at its list price, after the monthly credit. Delete everything with `az group delete -n rg-wasl-demo`.
