# Azure / Foundry deployment: handoff for a local agent session

**Status: designed, not started.** Nothing in this file has been deployed or tried on Azure. It was written on 17 Nov 2026 from
a read of the repository (`main` @ 4bfd661) and Microsoft's public docs. Labels used below: **[repo]** verified in this
repository, **[docs]** from a cited Microsoft page, **[inferred]** not verified.

Read `AGENTS.md`, then `docs/BACKEND-HANDOFF.md` and `docs/DEPLOY.md` before you change anything. All data is synthetic; keep
`WASL_SYNTHETIC_DATA_ONLY=1` set everywhere.

## 0. Settle these first: ask the human, do not assume

1. **Is "deploy on Azure Foundry" a real requirement?** "Foundry" appears nowhere in the repo. The public challenge page
   says no platform or model is mandatory, and Microsoft is listed only as a technology collaborator (`docs/CHALLENGE.md:51`).
   Ask the human where the requirement comes from, for example a judge, a track rule or a Microsoft credit offer. The answer decides
   between the minimum and the full path below.
2. **Deadline.** Submission closes **2026-11-20T18:29Z**, which is 22:29 UAE time on 20 Nov (`docs/CHALLENGE.md:3,281`) [repo]. Budget
   about 2.5 working days.
3. **Subscription, resource group and region.** Ask for:
   - a subscription;
   - one dedicated resource group, with the session's identity holding Contributor on that group only;
   - a region.

   UAE North is the natural choice. Check that the models and GPU SKUs exist there before committing to it.
4. **GPU quota.** Option A or B below needs GPU quota, which the human requests in the portal and which can take hours to days.
   Ask them to file it **first**.
5. **Cost approval.** Before creating any billable resource, show the human the resources and a rough monthly cost and get a yes.

## 1. What "the backend" is [repo]

| Component | Where | Runtime need |
|---|---|---|
| `wsi7 serve`: read and write API, port 8088 | `pipeline/`, image `wasl:local` (root `Dockerfile`) | Long-running. Reads a finished run under `/var/lib/wasl/run/<policy>-cached-<source>/`. Appends the hash-chained ledger `api_reviews.ndjson` under an in-process mutex plus `flock` (`pipeline/ledger_lock.go`), so it needs **exactly one replica**. Needs `WASL_API_TOKENS` (`token:role:actor`, comma-separated) and `WASL_SYNTHETIC_DATA_ONLY=1`. NATS is *not* required to serve (`api.go`: `not_required_for_read_api`). |
| Pipeline run (`scripts/demo.sh container`) | same image | **One-shot job.** Generates the synthetic cohort and runs ingest, prep ×3, linker, closure ×4, reviewer, audit and booking against NATS. Then it exits. Needs `WASL_NATS_URL` with `WASL_NATS_EXTERNAL=1`, both set together. |
| NATS JetStream ×3 | `deploy/nats/Dockerfile`, `deploy/nats/nats.conf` | Stateful. R3 streams, and some KV buckets are hard-coded to `Replicas: 3` (`fhirsub.go`, `fhirpoll.go`), so a single node will not work. Each node needs its own persistent volume. Needs `WASL_JS_KEY`, which must start with a letter; losing it makes the stores unreadable. |
| Evidence readers (Gemma 3 4B) | `pipeline/reader.go`, `drread.go`, `breastread.go`, `lungread.go`, `ldctread.go`, `ifhasread.go` | Default `WASL_READER=cached` replays saved answers and makes **no model calls**. Live mode calls an OpenAI-compatible `<url>/v1/chat/completions` served by `llama-server -m gemma-3-4b-it-Q4_K_M.gguf --port 8791 -c 8192 -ngl 99 -np 4`, with a JSON schema in `response_format`. The Go readers send **no auth header**. |
| Evidence investigator agent | `scripts/agent_investigator.py`, `scripts/agent_evidence.py`, `wsi7 agentshadow` (`docs/AGENT-SHADOW.md`) | Spike, simulated cohort only, not in the product path. Reads `WASL_LLM_URL`, `WASL_LLM_KEY` and `WASL_LLM_MODEL`, POSTs to `<url>/v1/chat/completions` with `Authorization: Bearer <key>` and `response_format: json_object`. Originally run against Gemini, so any other model needs re-validation. |
| Web app | `web/` (Next.js) | Designed for Vercel. It calls the API through `WASL_API_URL` plus role tokens. OIDC sessions are kept in SQLite for one instance, or in NATS KV for replicas. |

Local reference stack: `scripts/deploy_up.sh`, which runs `docker compose up --build`, then
`curl 127.0.0.1:8088/healthz` (`docs/DEPLOY.md`).

## 2. What Foundry can and cannot host [docs]

- **Hosted agents** are Python or C# containers that speak the Responses or Invocations protocol. Each session is sandboxed
  at **2 vCPU / 4 GiB** and released after an idle timeout
  (https://learn.microsoft.com/en-us/azure/foundry/agents/concepts/hosted-agents). They **cannot** run the Go API or a NATS
  cluster.
- **Model deployments** expose an OpenAI-compatible `/openai/v1/chat/completions`
  (https://learn.microsoft.com/en-us/azure/foundry/openai/api-version-lifecycle). Hugging Face models such as Gemma can run on
  **managed compute**, which is in preview and needs GPU quota
  (https://huggingface.co/docs/microsoft-azure/guides/managed-compute-foundry).
- So the Go API, the pipeline and NATS run **alongside** Foundry: on Azure Container Apps (ACA), with AKS for NATS on the full
  path. Foundry hosts the model and the agent. [inferred]

## 3. Recommended plan

### Step 1: API on Azure Container Apps (no GPU; about 3–5 h) [inferred]
1. Create an ACR, then build and push the image: `az acr build -r <acr> -t wasl:<sha> .` from the repository root.
2. Produce a run directory **locally** with `scripts/deploy_up.sh`, copy `/var/lib/wasl` out of the `wasl-run` volume, and
   upload it to an Azure Files share.
3. Create a Container Apps environment and add the share as storage. Create an app from the image with:
   - min replicas = max replicas = 1;
   - single revision mode;
   - the share mounted at `/var/lib/wasl`;
   - args `["serve"]` with `WASL_API_ADDR=0.0.0.0:8088` (copy the `api` service in `docker-compose.yml` for the exact env).
4. Configure it:
   - put secrets in Key Vault or ACA secrets: `WASL_API_TOKENS` with fresh random tokens, never the published demo ones;
   - set `WASL_SYNTHETIC_DATA_ONLY=1`;
   - set `WASL_API_ORIGINS` to the web app's origin;
   - use HTTPS ingress.
5. Verify:
   - `GET /healthz`;
   - `GET /api/fixtures` with a bearer token;
   - one coordinator write followed by a reload, which proves the ledger appends and `flock` work on Azure Files.

   **Risk:** `flock` and fsync semantics on SMB or NFS Azure Files are unverified. If an append fails, consider a managed disk
   via AKS, or keep the ledger on the container's local disk for the demo and label it as such.
6. Point the web app's `WASL_API_URL` (Vercel or ACA) at the ACA URL.

Label this honestly in the docs: **the API is served on Azure; the pipeline run that produced its data was run locally.**

### Step 2: the model (pick one; ask the human)
- **A. Recommended: our exact model on ACA serverless GPU.** Package `llama-server` with `gemma-3-4b-it-Q4_K_M.gguf` (stored on
  Azure Files or Blob) and run it with the command above. Expose it internally only, or add auth. Because the model bytes
  match, the published results stay comparable. About 4–8 h, plus quota. Serverless GPU is only offered in some regions.
  [inferred]
- **B. Gemma from the Foundry catalogue on Foundry managed compute.** This is clearly "on Foundry", but it uses full-precision
  weights, not our Q4_K_M quantisation. Its outputs will differ from the cached answers, so any metric from it must not be
  mixed with the published results. Whether that endpoint supports JSON-schema `response_format` is unverified. About 6–12 h,
  plus quota.

### Step 3: the custom agent as a Foundry hosted agent (about 4–8 h) [inferred]
Wrap `scripts/agent_investigator.py`, or a thin agent that calls the Wasl read API with a read-only token, in a Python
container that implements the hosted-agent protocol. Set `WASL_LLM_URL`, `WASL_LLM_KEY` and `WASL_LLM_MODEL` to the model
from step 2. For a Foundry endpoint the base URL is probably `https://<resource>/openai` so the client's
`/v1/chat/completions` resolves correctly; confirm this against the endpoint. Keep the existing safety bounds in
`docs/AGENT-SHADOW.md`: call caps, the cache-first rule, and no writes to decisions.

### Full-fidelity path (about 45–70 h; only if the human insists)
Add, on top of the steps above:
- the pipeline as an ACA Job;
- three-node TLS NATS on AKS, as a StatefulSet with managed disks (`docs/RECOVERY.md` has the TLS and backup material);
- Key Vault for `WASL_JS_KEY` and the NATS TLS material;
- Entra ID as the web app's OIDC provider;
- live reader re-validation;
- Bicep or `azd` infrastructure as code.

This is not realistic before the deadline alongside the video and frontend work.

## 4. Code changes needed [repo]

1. **Optional auth header on the Go live readers.** In `pipeline/reader.go`, `drread.go`, `breastread.go`, `lungread.go`,
   `ldctread.go` and `ifhasread.go`, the request is a bare `cl.Post(url+"/v1/chat/completions", ...)`. Add an optional key
   from an environment variable, sent as `Authorization: Bearer` or `api-key`, and leave the request unchanged when it is
   unset. Add a unit test against an `httptest` server.
2. **`safeReaderModel` (`pipeline/api.go:329`) only discloses model names starting `gemma-`.** If option B uses another name,
   decide with the human whether to widen this or keep the `gemma-` deployment name.
3. **`deploy/azure/`**: Bicep or `azure.yaml` (azd), the ACA app and job definitions, Key Vault references, and a README.
   Never commit secrets or keys.
4. **`docs/DEPLOY.md`**: add an "Azure" section that labels each part **built**, **simulated** or **designed**, with the date
   and what was actually verified.

Follow the repo rules: append-only ledger, no clinical-rule changes, truthful labels, and tests for each behaviour. CI is
described in `docs/CI.md`; branch protection requires the branch to be up to date with `main` and the "CI required" check
to pass.

## 5. Other open work (context only)

- **#167, the patient contests.** Built and tested in-process. Not proven end to end: the `wsi7 patient-contest-feed`
  publisher has never run against live NATS, and there is no integrated run with contests. It also needs a clinical
  decision: a contest on the root lung loop does not hold the surveillance child.
- **#157:** needs human clinical sign-off and qualified translator review.
- **#94, the soak run:** `python3 scripts/soak.py smoke --engine docker` failed in the cloud container because Docker Hub
  returned 429 rate limits. On a local Docker host it should run as documented in `docs/SOAK.md`. Label any run shorter than
  24 h as such.
