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
for user in coordinator clinician reviewer auditor; do
  password="$(openssl rand -hex 24)"
  printf '%s\n' "$password" > "$dir/api_${user}_password"
  variable="WASL_BOOTSTRAP_$(printf '%s' "$user" | tr '[:lower:]' '[:upper:]')"
  export "$variable=$password"
done
WASL_BOOTSTRAP_DIR="$dir" python3 - <<'PY'
import json, os
from pathlib import Path
root = Path(os.environ["WASL_BOOTSTRAP_DIR"])
users = {role: {"password": os.environ["WASL_BOOTSTRAP_" + role.upper()], "role": role}
         for role in ("coordinator", "clinician", "reviewer", "auditor")}
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
