#!/usr/bin/env bash
# Store the Deploy workflow's three secrets in GitHub without printing them. Run interactively by the person who owns
# the Modal workspace and the Vercel team; the Azure IDs are repository variables (deploy/azure/README.md).
#   bash deploy/azure/ci_secrets.sh
set -euo pipefail
REPO="${REPO:-Mizore66/wasl-futurehealth}"
PROFILE=wasl-ci
export PYTHONIOENCODING=utf-8  # Modal's spinner crashes on a Windows cp1252 console otherwise

echo "1/2 Modal: a browser window opens. Approve the new token for your workspace."
python -m modal token new --profile "$PROFILE" --no-activate --expires-in 30d >/dev/null
# Copy the CI profile's token into GitHub, then drop it from ~/.modal.toml so no second copy stays on this machine.
python - "$REPO" "$PROFILE" <<'PY'
import os, re, subprocess, sys, tomllib
repo, profile = sys.argv[1], sys.argv[2]
path = os.path.expanduser("~/.modal.toml")
token = tomllib.load(open(path, "rb"))[profile]
for name, key in (("MODAL_TOKEN_ID", "token_id"), ("MODAL_TOKEN_SECRET", "token_secret")):
    subprocess.run(["gh", "secret", "set", name, "--repo", repo], input=token[key].encode(), check=True)
text = open(path, encoding="utf-8").read()
open(path, "w", encoding="utf-8").write(re.sub(rf"(?ms)^\[{re.escape(profile)}\]\n.*?(?=^\[|\Z)", "", text))
print("Modal token stored as MODAL_TOKEN_ID and MODAL_TOKEN_SECRET")
PY

echo
echo "2/2 Vercel: create a token at https://vercel.com/account/settings/tokens"
echo "    Scope: the kais-projects-42abda1c team. Expiration: 30 days."
explorer.exe "https://vercel.com/account/settings/tokens" 2>/dev/null || true
read -rsp "    Paste the token (input is hidden), then press Enter: " token
echo
test -n "$token" || { echo "no token entered" >&2; exit 1; }
printf '%s' "$token" | gh secret set VERCEL_TOKEN --repo "$REPO"
unset token
echo "Vercel token stored as VERCEL_TOKEN"

echo
gh secret list --repo "$REPO"
