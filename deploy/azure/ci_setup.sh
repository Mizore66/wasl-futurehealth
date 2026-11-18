#!/usr/bin/env bash
# One-time setup for .github/workflows/deploy.yml. Run by a repository admin (setting Actions variables and
# secrets needs admin). The IDs are not secrets, but the repository is public, so they live in repository
# variables instead of committed files. The Vercel token is read from stdin and never echoed.
#   AZURE_CLIENT_ID=... AZURE_TENANT_ID=... AZURE_SUBSCRIPTION_ID=... VERCEL_ORG_ID=... VERCEL_PROJECT_ID=... \
#     [MODAL_TOKEN_ID=... MODAL_TOKEN_SECRET=...] deploy/azure/ci_setup.sh < vercel-token.txt
set -euo pipefail
REPO="${REPO:-Mizore66/wasl-futurehealth}"
for v in AZURE_CLIENT_ID AZURE_TENANT_ID AZURE_SUBSCRIPTION_ID VERCEL_ORG_ID VERCEL_PROJECT_ID; do
  : "${!v:?set $v}"
  gh variable set "$v" --repo "$REPO" --body "${!v}"
done
token="$(cat)"
test -n "$token" || { echo "pipe the Vercel token on stdin" >&2; exit 1; }
printf '%s' "$token" | gh secret set VERCEL_TOKEN --repo "$REPO"
# Optional: the Modal job deploys deploy/modal/ only with a Modal API token (dashboard, Settings > API Tokens).
if [ -n "${MODAL_TOKEN_ID:-}" ]; then
  : "${MODAL_TOKEN_SECRET:?set MODAL_TOKEN_SECRET with MODAL_TOKEN_ID}"
  printf '%s' "$MODAL_TOKEN_ID" | gh secret set MODAL_TOKEN_ID --repo "$REPO"
  printf '%s' "$MODAL_TOKEN_SECRET" | gh secret set MODAL_TOKEN_SECRET --repo "$REPO"
fi
gh variable list --repo "$REPO"
gh secret list --repo "$REPO" | grep -E 'VERCEL_TOKEN|MODAL_TOKEN'
