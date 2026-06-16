#!/usr/bin/env bash
# End-to-end smoke test: run the built binary against the mock Jenkins
# server and assert the agent-facing contract holds (JSON output, structured
# errors, exit codes). No real credentials or server required.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/jenkins-cli"
ADDR="127.0.0.1:45080"
URL="http://$ADDR"

if [[ ! -x "$BIN" ]]; then
  echo "building binary..."
  (cd "$ROOT" && make build >/dev/null)
fi

TMP="$(mktemp -d)"
cleanup() {
  [[ -n "${MOCK_PID:-}" ]] && kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Build and start the mock server (build to a binary so the trap can kill the
# actual server process — `go run` would leave its compiled child orphaned).
go build -o "$TMP/mockserver" "$ROOT/test/mockserver"
"$TMP/mockserver" "$ADDR" 2>"$TMP/mock.log" &
MOCK_PID=$!

# Wait for it to accept connections.
for _ in $(seq 1 50); do
  if curl -fsS -o /dev/null "$URL/api/json" -H 'Authorization: Basic x' 2>/dev/null; then
    break
  fi
  sleep 0.1
done

export JENKINS_URL="$URL"
export JENKINS_USER="alice"
export JENKINS_TOKEN="tok"

run() { "$BIN" --config "$TMP" "$@"; }

pass=0
check() { # check <label> <expected-substr> -- <command...>
  local label="$1" want="$2"; shift 3
  local out; out="$("$@" 2>/dev/null || true)"
  if grep -q "$want" <<<"$out"; then
    echo "ok   - $label"
    pass=$((pass + 1))
  else
    echo "FAIL - $label (wanted: $want)"
    echo "$out" | head -5
    exit 1
  fi
}

check "job list"        '"name": "app"'        -- run job list
check "job list status" '"status": "failure"'  -- run job list
check "job get params"  '"BRANCH"'             -- run job get app
check "build list"      '"number": 7'          -- run build list app
check "build get"       '"result": "FAILURE"'  -- run build get app lastFailed
check "build get cause" '"user_id": "alice"'   -- run build get app lastFailed
check "build log"       'FAIL: TestThing'      -- run build log app lastFailed
check "build log follow" 'Build step failed'   -- run build log app --follow
check "build stages"    '"name": "Test"'       -- run build stages app lastFailed
check "build tests"     '"TestThing"'          -- run build tests app lastFailed --failed-only
check "build changes"   '"break the test"'     -- run build changes app lastFailed
check "build artifacts" 'app.jar'              -- run build artifacts app lastFailed
check "queue list"      '"id": 51'             -- run queue list
check "doctor healthy"  '"healthy": true'      -- run doctor --no-update-check
check "auth status"     '"authenticated": true' -- run auth status

# Writes: --dry-run previews without sending; read-only blocks the real write.
check "job build dry-run" '"dry_run": true'     -- run job build app --param BRANCH=main --dry-run
check "dry-run endpoint"  'buildWithParameters'  -- run job build app --param BRANCH=main --dry-run
check "build stop dry-run" '/job/app/7/stop'     -- run build stop app 7 --dry-run

# Read-only mode blocks a real write with exit code 5 (permission).
set +e
JENKINS_CLI_READ_ONLY=1 run job build app >/dev/null 2>&1; code=$?
set -e
if [[ "$code" -ne 5 ]]; then
  echo "FAIL - read-only write should exit 5, got $code"; exit 1
fi
echo "ok   - read-only blocks job build (exit 5)"
pass=$((pass + 1))

# --allow-writes overrides read-only and actually triggers (mock returns a queue ref).
check "allow-writes triggers" '"triggered": true' -- run --allow-writes job build app

# Exit-code contract: missing required path arg -> usage (2).
set +e
run build get >/dev/null 2>&1; code=$?
set -e
if [[ "$code" -ne 2 ]]; then
  echo "FAIL - missing path arg should exit 2, got $code"; exit 1
fi
echo "ok   - missing-arg exits 2"
pass=$((pass + 1))

# config init: a fresh setup, then a re-run that must announce the existing
# config and ask edit/add/replace — here driving the "add" path to a 2nd context.
CFG="$(mktemp -d)"
printf '%s\nalice\ntoken\ntok\n' "$URL" | "$BIN" --config "$CFG" config init >/dev/null 2>&1
printf 'add\nprod\n%s\nbob\ntoken\ntok\n' "$URL" | "$BIN" --config "$CFG" config init >/dev/null 2>"$CFG/err"
if grep -q "edit/add/replace" "$CFG/err" \
   && "$BIN" --config "$CFG" config contexts 2>/dev/null | grep -q '"name": "prod"'; then
  echo "ok   - config init asks edit/add/replace and adds a context"
  pass=$((pass + 1))
else
  echo "FAIL - config init add-context flow"; cat "$CFG/err" | head -5; exit 1
fi
rm -rf "$CFG"

echo ""
echo "e2e: $pass checks passed"
