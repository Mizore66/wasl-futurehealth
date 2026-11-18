#!/usr/bin/env bash
# Store the Deploy workflow's three secrets in GitHub without printing them. Run interactively by the person who owns
# the Modal workspace and the Vercel team; the Azure IDs are repository variables (deploy/azure/README.md).
#   bash deploy/azure/ci_secrets.sh            # Modal and Vercel
#   bash deploy/azure/ci_secrets.sh vercel     # only one of them: modal | vercel
set -euo pipefail
REPO="${REPO:-Mizore66/wasl-futurehealth}"
TEAM=kais-projects-42abda1c
PROFILE=wasl-ci
ONLY="${1:-all}"
export PYTHONIOENCODING=utf-8  # Modal's spinner crashes on a Windows cp1252 console otherwise

if [ "$ONLY" = all ] || [ "$ONLY" = modal ]; then
  echo "Modal: a browser window opens. Approve the new token for your workspace."
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
fi

if [ "$ONLY" = all ] || [ "$ONLY" = vercel ]; then
  echo "Vercel: create a token at https://vercel.com/account/settings/tokens"
  echo "    Scope: $TEAM. Expiration: 30 days. Copy it right after creating it; Vercel shows it once."
  explorer.exe "https://vercel.com/account/settings/tokens" 2>/dev/null || true
  read -rsp "    Paste the token (input is hidden), then press Enter: " token
  echo
  token="${token//[$'\r\n\t ']/}"
  test -n "$token" || { echo "no token entered" >&2; exit 1; }
  # Check the token before storing it: a bad paste or a personal-scope token fails here, not in the Deploy run.
  if ! account="$(vercel whoami --token="$token" 2>/dev/null)"; then
    echo "Vercel rejected that token (invalid, deleted, or not the token text). Nothing was stored." >&2
    exit 1
  fi
  if ! vercel teams ls --token="$token" 2>/dev/null | grep -q "$TEAM"; then
    echo "The token works for $account but cannot see $TEAM. Recreate it with that scope. Nothing was stored." >&2
    exit 1
  fi
  printf '%s' "$token" | gh secret set VERCEL_TOKEN --repo "$REPO"
  unset token
  echo "Vercel token for $account, valid for $TEAM, stored as VERCEL_TOKEN"
fi

echo
gh secret list --repo "$REPO"
