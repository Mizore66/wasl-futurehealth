#!/usr/bin/env bash
# One-time setup for .github/workflows/deploy.yml, and the command to rerun when its tokens expire. Needs `az` and
# `vercel link` on this machine and a GitHub account allowed to set the repository's Actions variables and secrets.
# The IDs are not secrets, but the repository is public, so they live in repository variables, not committed files.
#   bash deploy/azure/ci_setup.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
REPO="${REPO:-Mizore66/wasl-futurehealth}"
set -a; . deploy/azure/ci.env; set +a

gh variable set AZURE_CLIENT_ID --repo "$REPO" \
  --body "$(az identity show -g "$WASL_RG" -n wasl-github-deploy --query clientId -o tsv)"
gh variable set AZURE_TENANT_ID --repo "$REPO" --body "$(az account show --query tenantId -o tsv)"
gh variable set AZURE_SUBSCRIPTION_ID --repo "$REPO" --body "$(az account show --query id -o tsv)"
for v in orgId:VERCEL_ORG_ID projectId:VERCEL_PROJECT_ID; do
  gh variable set "${v#*:}" --repo "$REPO" \
    --body "$(python -c "import json; print(json.load(open('.vercel/project.json'))['${v%%:*}'])")"
done
gh variable list --repo "$REPO"

exec bash deploy/azure/ci_secrets.sh
