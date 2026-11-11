#!/usr/bin/env bash
# Generate a new, external secret directory. Existing directories are never overwritten and secret values are never printed.
# The directory itself is 0700. Individual files are bind-mounted into containers; non-root readers require 0444 files.
set -euo pipefail
if [ "$#" -ne 1 ]; then
  echo "usage: $0 <new-empty-secret-directory>" >&2
  exit 2
fi
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
dir="$1"
if [ -e "$dir" ]; then
  echo "refusing to overwrite existing secret path: $dir" >&2
  exit 1
fi
umask 077
mkdir -m 0700 "$dir"
# Fail cleanup removes an incomplete generation, but never an existing directory.
trap 'rc=$?; echo "secret generation failed at line $LINENO (exit $rc)" >&2; rm -rf "$dir"' ERR
js_key="k$(openssl rand -hex 32)"
nats_password="$(openssl rand -hex 32)"
route_password="$(openssl rand -hex 32)"
backup_key="$(openssl rand -hex 32)"
printf '%s\n' "$js_key" > "$dir/js_key"
printf '%s\n' "$nats_password" > "$dir/nats_password"
printf '%s\n' "$route_password" > "$dir/route_password"
printf '%s\n' "$backup_key" > "$dir/backup_key"
# Versioned recovery keyring for encrypted v2 bundles; generated in Python so no key reaches argv, stdout or logs.
# `backup_key` above is kept only for explicitly opted-in legacy HMAC bundles.
WASL_KEYRING_PATH="$dir/recovery_keys.json" python3 - <<'PY'
import base64, json, os
key_id = "g-" + os.urandom(8).hex()
entry = {name: base64.b64encode(os.urandom(32)).decode() for name in ("wrap_key", "auth_key")}
doc = {"schema": "wasl-recovery-keyring-v1", "active": key_id, "keys": {key_id: entry}}
fd = os.open(os.environ["WASL_KEYRING_PATH"], os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, "w") as handle:
    handle.write(json.dumps(doc, sort_keys=True, indent=2) + "\n")
PY
# Distinct credentials for the independent double-review protocol (wasl-independent-review/1): two ordinary
# initial reviewers and clinicians plus one senior of each discipline. Names are the API actor identities. The first
# reviewer/clinician keep their original names and password files (the health check reads them); the added identities
# live only in api_users.json. Nothing is printed.
specs=""
for entry in coordinator:coordinator clinician:clinician reviewer:reviewer reviewer-2:reviewer senior-reviewer:senior_reviewer \
             clinician-2:clinician senior-clinician:senior_clinician auditor:auditor; do
  user="${entry%%:*}"; role="${entry#*:}"
  password="$(openssl rand -hex 24)"
  slug="$(printf '%s' "$user" | tr '-' '_')"
  case "$user" in coordinator|clinician|reviewer|auditor) printf '%s\n' "$password" > "$dir/api_${slug}_password";; esac
  variable="WASL_BOOTSTRAP_$(printf '%s' "$slug" | tr '[:lower:]' '[:upper:]')"
  export "$variable=$password"
  specs="$specs $user:$role:$variable"
done
WASL_BOOTSTRAP_DIR="$dir" WASL_BOOTSTRAP_SPECS="$specs" python3 - <<'PY'
import json, os
from pathlib import Path
root = Path(os.environ["WASL_BOOTSTRAP_DIR"])
users = {}
for spec in os.environ["WASL_BOOTSTRAP_SPECS"].split():
    name, role, variable = spec.split(":")
    users[name] = {"password": os.environ[variable], "role": role}
if len({u["password"] for u in users.values()}) != len(users) or len({n.lower() for n in users}) != len(users):
    raise SystemExit("bootstrap credentials are not distinct")
(root / "api_users.json").write_text(json.dumps(users, sort_keys=True) + "\n")
PY
openssl req -x509 -newkey rsa:3072 -sha256 -days 3650 -nodes -subj '/CN=Wasl recovery test CA' \
  -addext 'basicConstraints=critical,CA:TRUE' -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -addext 'subjectKeyIdentifier=hash' \
  -keyout "$dir/ca.key" -out "$dir/ca.crt" >/dev/null 2>&1
cat > "$dir/openssl.cnf" <<'EOF'
[req]
prompt = no
distinguished_name = dn
req_extensions = reqext
[dn]
CN = wasl-services
[reqext]
subjectAltName = DNS:localhost,DNS:nats-0,DNS:nats-1,DNS:nats-2,DNS:api,IP:127.0.0.1
[ext]
subjectAltName = DNS:localhost,DNS:nats-0,DNS:nats-1,DNS:nats-2,DNS:api,IP:127.0.0.1
extendedKeyUsage = serverAuth,clientAuth
keyUsage = digitalSignature,keyEncipherment
basicConstraints = critical,CA:FALSE
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
EOF
openssl req -new -newkey rsa:3072 -sha256 -nodes -config "$dir/openssl.cnf" \
  -keyout "$dir/server.key" -out "$dir/server.csr" >/dev/null 2>&1
openssl x509 -req -in "$dir/server.csr" -CA "$dir/ca.crt" -CAkey "$dir/ca.key" -CAcreateserial \
  -days 825 -sha256 -extensions ext -extfile "$dir/openssl.cnf" -out "$dir/server.crt" >/dev/null
rm -f "$dir/openssl.cnf" "$dir/server.csr" "$dir/ca.srl"
# The host directory prevents unrelated host users from traversing these paths. Compose bind-mounts individual files,
# and container UID 1000/10001 needs read access without running the service as root.
chmod 0444 "$dir"/*
chmod 0600 "$dir/ca.key"
trap - ERR
printf '{"level":"INFO","msg":"secret files generated; values not logged","directory":"%s"}\n' "$dir"
