# Wasl audit explainer (Foundry hosted agent)

A read-only demo agent that explains one Wasl case's redacted audit trail to an auditor in plain language. All data is synthetic.

Status, 17 Nov 2026: **built** and deployed as a Foundry hosted agent. One end-to-end call was observed. It read the token from Key Vault, read the Azure-hosted API and called `gpt-4.1-mini`. Its output is unvalidated model prose: in that call it said the case closed "after about 50 days, which was within the 31-day" window. Treat every answer as a summary to check against the audit, never as a finding. See [`docs/DEPLOY.md`, "Azure"](../../../docs/DEPLOY.md#azure).

## What it does

1. Extracts one case id from the message, for example `ahs-mushrif/ServiceRequest/AHS-sr-1001`. With no id it asks for one and makes no API or model call.
2. Sends one `GET {WASL_API_URL}/api/cases/{id}/audit` with the auditor bearer token. It never calls a POST route. A rejection or timeout returns a short error with the HTTP status, never the token.
3. Builds a bounded prompt (verification flag, signature summary, the last 12 chain events with trimmed bodies, at most 6000 characters) and makes one chat-completion call with `max_tokens=500`.
4. Replies with a provenance line (case id, `chain_verified`, model deployment) followed by the model's explanation.

## Environment

| Variable | Required | Meaning |
|---|---|---|
| `WASL_API_URL` | yes | Base URL of the Wasl API |
| `WASL_KEYVAULT_URL` | no | Key Vault holding the auditor token. When set, the token is read once with `DefaultAzureCredential` |
| `WASL_AUDITOR_SECRET_NAME` | no | Secret name, default `wasl-auditor-token` |
| `WASL_API_TOKEN` | local dev only | Auditor token used when `WASL_KEYVAULT_URL` is unset |
| `FOUNDRY_PROJECT_ENDPOINT` | yes | Foundry project endpoint, injected by Foundry |
| `AZURE_AI_MODEL_DEPLOYMENT_NAME` | no | Chat model deployment, default `gpt-4.1-mini` |

## Local run

```bash
python -m venv .venv
.venv/Scripts/python -m pip install -r requirements.txt pytest
.venv/Scripts/python -m pytest -q tests

WASL_API_URL=http://127.0.0.1:8088 WASL_API_TOKEN=wasl-demo-auditor-token \
FOUNDRY_PROJECT_ENDPOINT=https://<resource>.services.ai.azure.com/api/projects/<project> \
.venv/Scripts/python main.py   # serves POST /responses on PORT (default 8088)

curl -X POST http://127.0.0.1:8088/responses -H 'Content-Type: application/json' \
  -d '{"input":"Explain ahs-mushrif/ServiceRequest/AHS-sr-1001"}'
```

The agent server defaults to port 8088, the same port as `scripts/demo.sh serve`. Run one of them on another port (`PORT` for the agent, `WASL_API_ADDR` for the API) when both run locally.
