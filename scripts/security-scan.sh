#!/usr/bin/env bash
# Security scan driver — runs OWASP ZAP and nuclei
# against the motus dev stack running on this host.
# SAST (gosec, govulncheck, semgrep) runs in .github/workflows/security.yaml.
#
# Usage: ./scripts/security-scan.sh [--skip-stack]
#   --skip-stack   Skip docker compose up (stack already running)
#
# Output: security-reports/<timestamp>/{zap-api,zap-full,nuclei,summary}.{html,json,txt,md}
#
# Run on the remote dev host (root@<REMOTE_HOST>) from /root/motus/:
#   ssh root@<REMOTE_HOST> 'cd /root/motus && bash scripts/security-scan.sh'
#
# Rsync to remote with --exclude='.envrc' to avoid copying local credentials.

set -euo pipefail

TARGET_URL="http://localhost:8080"
ADMIN_EMAIL="admin@motus.local"
ADMIN_PASS="${ADMIN_PASS:-admin}"
TS=$(date +%Y%m%dT%H%M%S)
REPORT_DIR="security-reports/${TS}"
SKIP_STACK=false

for arg in "$@"; do
  [[ "$arg" == "--skip-stack" ]] && SKIP_STACK=true
done

mkdir -p "${REPORT_DIR}"
# ZAP container runs as UID 1000 (non-root); the mounted host dir must be world-writable
chmod 777 "${REPORT_DIR}"
echo "Reports → ${REPORT_DIR}"

# ─────────────────────────────────────────────
# 1. Stack
# ─────────────────────────────────────────────
if [[ "${SKIP_STACK}" == "false" ]]; then
  echo "▸ Starting stack (docker compose dev)..."
  docker compose -f docker-compose.yaml -f docker-compose.dev.yaml up -d --build --wait
fi

echo "▸ Waiting for health endpoint..."
for i in $(seq 1 30); do
  if curl -sf "${TARGET_URL}/api/health" >/dev/null 2>&1; then
    echo "  Stack is healthy."
    break
  fi
  sleep 2
  [[ $i -eq 30 ]] && { echo "ERROR: stack never became healthy"; exit 1; }
done

# ─────────────────────────────────────────────
# 2. Mint a temporary bearer API key
# ─────────────────────────────────────────────
echo "▸ Minting scan API key..."

LOGIN_RESP=$(curl -sf -X POST "${TARGET_URL}/api/session" \
  -H "Content-Type: application/json" \
  -c /tmp/motus-scan-cookies.txt \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASS}\"}" \
  -D /tmp/motus-scan-headers.txt)

# The login endpoint is CSRF-exempt (UnsafeSkipCheck), so X-CSRF-Token is empty
# in the login response. Use X-Auth-Token (= session_id value) for subsequent
# requests — the CSRF middleware validates it against a live session and exempts it.
SESSION_COOKIE=$(grep 'session_id' /tmp/motus-scan-cookies.txt | awk '{print $NF}')

if [[ -z "${SESSION_COOKIE}" ]]; then
  echo "ERROR: failed to log in — no session_id cookie (check admin creds)"
  exit 1
fi

APIKEY_RESP=$(curl -sf -X POST "${TARGET_URL}/api/keys" \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: ${SESSION_COOKIE}" \
  -d '{"name":"security-scan-key","permissions":"readonly"}')

BEARER_TOKEN=$(echo "${APIKEY_RESP}" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
APIKEY_ID=$(echo "${APIKEY_RESP}" | grep -o '"id":[0-9]*' | cut -d: -f2)
if [[ -z "${BEARER_TOKEN}" ]]; then
  echo "ERROR: failed to create API key (response: ${APIKEY_RESP})"
  exit 1
fi
echo "  API key created (readonly bearer)."

# ─────────────────────────────────────────────
# 3. ZAP — OpenAPI-driven API scan
# ─────────────────────────────────────────────
echo "▸ ZAP API scan (OpenAPI-driven)..."
docker run --rm --network host \
  -v "${PWD}/${REPORT_DIR}:/zap/wrk/out" \
  ghcr.io/zaproxy/zaproxy:stable \
  zap-api-scan.py \
    -t "${TARGET_URL}/api/docs/openapi.yaml" \
    -f openapi \
    -z "-config replacer.full_list(0).description=bearer_auth \
        -config replacer.full_list(0).enabled=true \
        -config replacer.full_list(0).matchtype=REQ_HEADER \
        -config replacer.full_list(0).matchstr=Authorization \
        -config replacer.full_list(0).replacement=Bearer_${BEARER_TOKEN}" \
    -r "/zap/wrk/out/zap-api.html" \
    -J "/zap/wrk/out/zap-api.json" \
    -w "/zap/wrk/out/zap-api.md" \
    -I || true  # -I: don't exit non-zero on alerts; we triage manually

# ─────────────────────────────────────────────
# 4. ZAP — full spider + active scan
# ─────────────────────────────────────────────
echo "▸ ZAP full scan (spider + active)..."
docker run --rm --network host \
  -v "${PWD}/${REPORT_DIR}:/zap/wrk/out" \
  ghcr.io/zaproxy/zaproxy:stable \
  zap-full-scan.py \
    -t "${TARGET_URL}" \
    -z "-config replacer.full_list(0).description=bearer_auth \
        -config replacer.full_list(0).enabled=true \
        -config replacer.full_list(0).matchtype=REQ_HEADER \
        -config replacer.full_list(0).matchstr=Authorization \
        -config replacer.full_list(0).replacement=Bearer_${BEARER_TOKEN}" \
    -r "/zap/wrk/out/zap-full.html" \
    -J "/zap/wrk/out/zap-full.json" \
    -w "/zap/wrk/out/zap-full.md" \
    -I || true

# ─────────────────────────────────────────────
# 5. nuclei — web vuln templates
# ─────────────────────────────────────────────
echo "▸ nuclei (web vuln templates)..."
docker run --rm --network host \
  -v "${PWD}/${REPORT_DIR}:/work" \
  projectdiscovery/nuclei:latest \
  -u "${TARGET_URL}" \
  -severity medium,high,critical \
  -H "Authorization: Bearer ${BEARER_TOKEN}" \
  -je "/work/nuclei.json" \
  -o "/work/nuclei.txt" \
  -stats || true

# ─────────────────────────────────────────────
# 6. Summary
# ─────────────────────────────────────────────
echo "▸ Generating summary..."

count_zap_api="?"
count_zap_full="?"
count_nuclei="0"

[[ -f "${REPORT_DIR}/zap-api.json" ]] && \
  count_zap_api=$(python3 -c "
import json, sys
d = json.load(open('${REPORT_DIR}/zap-api.json'))
print(sum(len(s.get('instances',[])) for r in d.get('site',[]) for s in r.get('alerts',[])))
" 2>/dev/null || echo "?")

[[ -f "${REPORT_DIR}/zap-full.json" ]] && \
  count_zap_full=$(python3 -c "
import json, sys
d = json.load(open('${REPORT_DIR}/zap-full.json'))
print(sum(len(s.get('instances',[])) for r in d.get('site',[]) for s in r.get('alerts',[])))
" 2>/dev/null || echo "?")

[[ -f "${REPORT_DIR}/nuclei.json" ]] && \
  count_nuclei=$(wc -l < "${REPORT_DIR}/nuclei.json" 2>/dev/null | tr -d ' ' || echo "0")

cat > "${REPORT_DIR}/summary.md" <<EOF
# motus Security Scan — ${TS}

## Finding Counts

| Tool | Type | Findings | Report |
|------|------|----------|--------|
| ZAP API scan | DAST (OpenAPI) | ${count_zap_api} instances | zap-api.html / zap-api.json |
| ZAP full scan | DAST (spider+active) | ${count_zap_full} instances | zap-full.html / zap-full.json |
| nuclei | Web vuln templates | ${count_nuclei} findings | nuclei.json / nuclei.txt |

## Scope

- Target: ${TARGET_URL}
- Stack: docker-compose.yaml + docker-compose.dev.yaml (rate limits relaxed: 1000/10000)
- Auth: readonly Bearer API key (rotate after scan)
- Deferred: trivy (container CVEs), GPS TCP port fuzzing (5013/5093)

## Next Steps

1. Triage each Medium+ finding and fill in triage.md:
   - fix-now: Critical/High
   - fix-later: Medium
   - false-positive: document reason, add to per-tool ignore file
2. If findings exist: consider a zap-baseline job in e2e.yaml
EOF

echo ""
echo "✓ Done. Reports in ${REPORT_DIR}/"
echo "  Summary: ${REPORT_DIR}/summary.md"
echo ""
echo "⚠  Rotate the scan API key when finished reviewing reports:"
echo "   SESSION=\$(curl -sf -c /tmp/c.txt -X POST ${TARGET_URL}/api/session -H 'Content-Type: application/json' -d '{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASS}\"}' -o /dev/null && grep session_id /tmp/c.txt | awk '{print \$NF}')"
echo "   curl -X DELETE ${TARGET_URL}/api/keys/${APIKEY_ID} -H \"X-Auth-Token: \${SESSION}\""
