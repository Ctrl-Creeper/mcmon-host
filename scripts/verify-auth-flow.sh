#!/bin/sh
set -eu

HOST_REPO="$(cd "$(dirname "$0")/.." && pwd)"
APP_REPO="$(cd "$HOST_REPO/../mc-latency-monitor" && pwd)"
ROOT="$(mktemp -d)"
cleanup() {
  if [ -n "${HOST_PID:-}" ]; then kill "$HOST_PID" 2>/dev/null || true; wait "$HOST_PID" 2>/dev/null || true; fi
  if [ -n "${APP_PID:-}" ]; then kill "$APP_PID" 2>/dev/null || true; wait "$APP_PID" 2>/dev/null || true; fi
  rm -rf "$ROOT"
}
trap cleanup EXIT

require_json_field() {
  file="$1"
  field="$2"
  if ! grep -q "\"$field\"" "$file"; then
    echo "missing $field in $file" >&2
    cat "$file" >&2
    exit 1
  fi
}

extract_json_string() {
  field="$1"
  sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" | head -n 1
}

wait_http() {
  url="$1"
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fsS "$url" >/dev/null 2>&1; then return 0; fi
    sleep 0.3
  done
  echo "timeout waiting for $url" >&2
  return 1
}

echo "== Go tests =="
(cd "$HOST_REPO" && go test ./...)
(cd "$APP_REPO" && go test ./...)
HOST_BIN="$ROOT/mcmon-host"
APP_BIN="$ROOT/mcmon"
(cd "$HOST_REPO" && go build -o "$HOST_BIN" ./cmd/mcmon-host)
(cd "$APP_REPO" && go build -o "$APP_BIN" ./cmd/mcmon)

echo "== Fresh host config generation =="
fresh="$ROOT/fresh"
mkdir -p "$fresh"
"$HOST_BIN" -config "$fresh/config.json" >"$fresh/host.log" 2>&1 &
HOST_PID=$!
wait_http http://127.0.0.1:9090/
kill "$HOST_PID" 2>/dev/null || true
wait "$HOST_PID" 2>/dev/null || true
HOST_PID=""
require_json_field "$fresh/config.json" admin_username
require_json_field "$fresh/config.json" admin_password
fresh_user="$(extract_json_string admin_username <"$fresh/config.json")"
fresh_pass="$(extract_json_string admin_password <"$fresh/config.json")"

echo "== Fresh config login =="
"$HOST_BIN" -config "$fresh/config.json" >"$fresh/host2.log" 2>&1 &
HOST_PID=$!
wait_http http://127.0.0.1:9090/
login="$(curl -fsS -X POST -H 'Content-Type: application/json' --data "{\"username\":\"$fresh_user\",\"password\":\"$fresh_pass\"}" http://127.0.0.1:9090/api/auth/login)"
session="$(printf '%s' "$login" | extract_json_string session_token)"
test -n "$session"
curl -fsS -H "Authorization: Bearer $session" http://127.0.0.1:9090/api/agents >/dev/null
kill "$HOST_PID" 2>/dev/null || true
wait "$HOST_PID" 2>/dev/null || true
HOST_PID=""

echo "== Legacy config migration =="
legacy="$ROOT/legacy"
mkdir -p "$legacy"
cat >"$legacy/config.json" <<EOF
{
  "listen": "127.0.0.1:19094",
  "db_path": "$legacy/host.db",
  "discovery_key": "discover",
  "admin_token": "legacy-token"
}
EOF
"$HOST_BIN" -config "$legacy/config.json" >"$legacy/host.log" 2>&1 &
HOST_PID=$!
wait_http http://127.0.0.1:19094/
kill "$HOST_PID" 2>/dev/null || true
wait "$HOST_PID" 2>/dev/null || true
HOST_PID=""
require_json_field "$legacy/config.json" admin_username
require_json_field "$legacy/config.json" admin_password
legacy_pass="$(extract_json_string admin_password <"$legacy/config.json")"

echo "== Config password change survives restart =="
python3 - "$legacy/config.json" <<'PY'
import json, sys
path = sys.argv[1]
data = json.load(open(path))
data["admin_username"] = "changed"
data["admin_password"] = "changed-password"
json.dump(data, open(path, "w"), indent=2)
PY
"$HOST_BIN" -config "$legacy/config.json" >"$legacy/host2.log" 2>&1 &
HOST_PID=$!
wait_http http://127.0.0.1:19094/
curl -fsS -X POST -H 'Content-Type: application/json' --data '{"username":"changed","password":"changed-password"}' http://127.0.0.1:19094/api/auth/login >/dev/null
if curl -fsS -X POST -H 'Content-Type: application/json' --data "{\"username\":\"admin\",\"password\":\"$legacy_pass\"}" http://127.0.0.1:19094/api/auth/login >/dev/null 2>&1; then
  echo "old password still works after config change" >&2
  exit 1
fi

echo "== 2FA setup and login =="
session="$(curl -fsS -X POST -H 'Content-Type: application/json' --data '{"username":"changed","password":"changed-password"}' http://127.0.0.1:19094/api/auth/login | extract_json_string session_token)"
setup="$(curl -fsS -H "Authorization: Bearer $session" http://127.0.0.1:19094/api/auth/2fa/setup)"
secret="$(printf '%s' "$setup" | extract_json_string secret)"
code="$(python3 - "$secret" <<'PY'
import hmac, base64, hashlib, struct, sys, time
secret = sys.argv[1].replace(" ", "")
padding = "=" * ((8 - len(secret) % 8) % 8)
key = base64.b32decode(secret.upper() + padding)
counter = int(time.time() // 30)
msg = struct.pack(">Q", counter)
digest = hmac.new(key, msg, hashlib.sha1).digest()
offset = digest[-1] & 0x0F
code = (struct.unpack(">I", digest[offset:offset+4])[0] & 0x7fffffff) % 1000000
print(f"{code:06d}")
PY
)"
curl -fsS -X POST -H "Authorization: Bearer $session" -H 'Content-Type: application/json' --data "{\"secret\":\"$secret\",\"totp_code\":\"$code\"}" http://127.0.0.1:19094/api/auth/2fa/enable >/dev/null
if curl -fsS -X POST -H 'Content-Type: application/json' --data '{"username":"changed","password":"changed-password"}' http://127.0.0.1:19094/api/auth/login >/dev/null 2>&1; then
  echo "login without 2FA unexpectedly succeeded" >&2
  exit 1
fi
curl -fsS -X POST -H 'Content-Type: application/json' --data "{\"username\":\"changed\",\"password\":\"changed-password\",\"totp_code\":\"$code\"}" http://127.0.0.1:19094/api/auth/login >/dev/null

echo "== Desktop app remote login =="
app="$ROOT/app"
mkdir -p "$app"
cat >"$app/config.json" <<EOF
{"listen_addr":"127.0.0.1:18094","db_path":"$app/mcmon.db","targets":[]}
EOF
"$APP_BIN" --config "$app/config.json" >"$app/app.log" 2>&1 &
APP_PID=$!
wait_http http://127.0.0.1:18094/
curl -fsS -X POST -H 'Content-Type: application/json' --data "{\"host_url\":\"http://127.0.0.1:19094\",\"username\":\"changed\",\"password\":\"changed-password\",\"totp_code\":\"$code\"}" http://127.0.0.1:18094/api/remote/login >/dev/null
curl -fsS http://127.0.0.1:18094/api/remote/agents >/dev/null
kill "$APP_PID" 2>/dev/null || true
wait "$APP_PID" 2>/dev/null || true
APP_PID=""
kill "$HOST_PID" 2>/dev/null || true
wait "$HOST_PID" 2>/dev/null || true
HOST_PID=""

echo "== install.sh config template =="
install_root="$ROOT/install-root"
mkdir -p "$install_root/etc/mcmon-host" "$install_root/var/lib/mcmon-host"
sh -n "$HOST_REPO/install.sh"
CONFIG_DIR="$install_root/etc/mcmon-host" DATA_DIR="$install_root/var/lib/mcmon-host" ADMIN_USERNAME="script-admin" ADMIN_PASSWORD="" MCMON_INSTALL_SH_SOURCE_ONLY=1 sh -c 'script="$1"; set --; . "$script"; ensure_config' sh "$HOST_REPO/install.sh"
require_json_field "$install_root/etc/mcmon-host/config.json" admin_username
require_json_field "$install_root/etc/mcmon-host/config.json" admin_password
script_user="$(extract_json_string admin_username <"$install_root/etc/mcmon-host/config.json")"
script_pass="$(extract_json_string admin_password <"$install_root/etc/mcmon-host/config.json")"
test "$script_user" = "script-admin"
test -n "$script_pass"

echo "verification ok"
