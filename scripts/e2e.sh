#!/usr/bin/env bash
# End-to-end smoke test: run the built binary against the mock Jenkins
# server and assert the agent-facing contract holds (JSON output, structured
# errors, exit codes). No real credentials or server required.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SKILL_VERSION="$(sed -n 's/^version: *//p' "$ROOT/skills/jenkins/SKILL.md" | head -1)"
BIN="$ROOT/bin/jenkins-cli"
ADDR="127.0.0.1:45080"
URL="http://$ADDR"

LDFLAGS="-X github.com/angelmsger/jenkins-cli/pkg/constants.Version=0.0.1"
(cd "$ROOT" && go build -ldflags "$LDFLAGS" -o "$BIN" ./cmd/jenkins-cli)

TMP="$(mktemp -d)"
cleanup() {
  [[ -n "${MOCK_PID:-}" ]] && kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$TMP"
}
trap cleanup EXIT

# Build and start the mock server (build to a binary so the trap can kill the
# actual server process — `go run` would leave its compiled child orphaned).
go build -o "$TMP/mockserver" "$ROOT/test/mockserver"
WRITE_LOG="$TMP/writes.log"
MOCK_WRITE_LOG="$WRITE_LOG" "$TMP/mockserver" "$ADDR" 2>"$TMP/mock.log" &
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
export JENKINS_RELEASE_API="$URL/releases/latest"
export JENKINS_CLI_NO_UPDATE_NOTIFIER=1
export JENKINS_CLI_SKILL="$SKILL_VERSION"

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
# --verbose logs one line per request (method, URL, status) on stderr: never on
# stdout, which stays data, and never a header.
verbose_out="$(JENKINS_URL="http://verbose-user:e2e-url-secret@${URL#http://}" run --verbose job list 2>"$TMP/verbose.err")"
if grep -Eq '^[A-Z]+ http://verbose-user:xxxxx@[^ ]+ -> 200$' "$TMP/verbose.err" &&
  grep -q '"name": "app"' <<<"$verbose_out" && ! grep -q ' -> 200' <<<"$verbose_out" &&
  ! grep -qF 'e2e-url-secret' "$TMP/verbose.err" &&
  ! grep -qF 'e2e-url-secret' <<<"$verbose_out" &&
  ! grep -qiE 'authorization|bearer |basic ' "$TMP/verbose.err"; then
  echo "ok   - --verbose logs on stderr only and masks the URL password"
  pass=$((pass + 1))
else
  echo "FAIL - --verbose logs on stderr only and masks the URL password"
  head -5 "$TMP/verbose.err"
  exit 1
fi
check "--verbose doctor logs its requests" ' -> 200' -- sh -c "\"$BIN\" --config \"$TMP\" --verbose doctor --no-update-check 2>&1 >/dev/null"
check "job list status" '"status": "failure"'  -- run job list
run --format ndjson --fields name job list >"$TMP/jobs.ndjson" 2>"$TMP/jobs.stderr"
if grep -qx '{"name":"app"}' "$TMP/jobs.ndjson" &&
   ! grep -q '"_notice"\|"has_more"\|"next"' "$TMP/jobs.ndjson" &&
   ! grep -q '"pagination"' "$TMP/jobs.stderr"; then
  echo "ok   - unpaginated NDJSON keeps projected rows and no continuation"
  pass=$((pass + 1))
else
  echo "FAIL - unpaginated NDJSON keeps projected rows and no continuation"; exit 1
fi
check "job get params"  '"BRANCH"'             -- run job get app
check "build list"      '"number": 7'          -- run build list app
check "build get"       '"result": "FAILURE"'  -- run build get app lastFailed
check "build get cause" '"user_id": "alice"'   -- run build get app lastFailed
check "build get commit" '"sha": "0123456789abcdef0123456789abcdef01234567"' -- run build get app lastFailed
check "build log"       'FAIL: TestThing'      -- run build log app lastFailed
check "build log follow" 'Build step failed'   -- run build log app 7 --follow
check "build stages"    '"name": "Test"'       -- run build stages app lastFailed
check "build tests"     '"TestThing"'          -- run build tests app lastFailed --failed-only
check "build changes"   '"break the test"'     -- run build changes app lastFailed
check "build artifacts" 'app.jar'              -- run build artifacts app lastFailed
check "queue list"      '"id": 51'             -- run queue list
check "queue assigned build" '"number": 8'    -- run queue get 77
check "queue build URL" 'http://x/job/app/8/'  -- run queue get 77
check "queue cancellation" '"cancelled": true' -- run queue get 78
check "doctor healthy"  '"healthy": true'      -- run doctor --no-update-check
check "doctor reports Skill" '"companion-skill"' -- run doctor --no-update-check
check "auth status"     '"authenticated": true' -- run auth status

SKILL_HOME="$TMP/skill-home"
mkdir -p "$SKILL_HOME"
check "skill install for Codex" '"alignment": "current"' -- \
  env HOME="$SKILL_HOME" "$BIN" --config "$TMP" skill install --agent codex
check "skill status version aligned" '"loaded_status": "current"' -- \
  env HOME="$SKILL_HOME" JENKINS_CLI_SKILL="$SKILL_VERSION" "$BIN" --config "$TMP" skill status
legacy_out="$(env HOME="$SKILL_HOME" JENKINS_CLI_SKILL=1 JENKINS_CLI_NO_UPDATE_NOTIFIER=1 \
  "$BIN" --config "$TMP" job list 2>&1 || true)"
if grep -q '"status":"unknown"' <<<"$legacy_out"; then
  echo "ok   - legacy Skill handshake is detected"
  pass=$((pass + 1))
else
  echo "FAIL - legacy Skill handshake is detected"; exit 1
fi
update_out="$(env -u JENKINS_CLI_NO_UPDATE_NOTIFIER JENKINS_CLI_SKILL="$SKILL_VERSION" \
  "$BIN" --config "$TMP" job list 2>&1 || true)"
if grep -q '"next_steps"' <<<"$update_out" && grep -q 'jenkins-cli skill install' <<<"$update_out"; then
  echo "ok   - update notice includes Skill refresh"
  pass=$((pass + 1))
else
  echo "FAIL - update notice includes Skill refresh"; exit 1
fi

# Writes: --dry-run previews without sending; read-only blocks the real write.
check "job build dry-run" '"dry_run": true'     -- run job build app --param BRANCH=main --dry-run
check "dry-run endpoint"  'buildWithParameters'  -- run job build app --param BRANCH=main --dry-run
check "build stop dry-run" '/job/app/7/stop'     -- run build stop app 7 --dry-run

# Read-only mode blocks every write command with READONLY_BLOCKED and exit code
# 5 (permission) before any request is sent. The mock server's write log must
# stay empty until the first --allow-writes call.
: >"$WRITE_LOG"
ro_blocks() { # ro_blocks <label> <command args...>
  local label="$1"; shift
  local err code
  set +e
  err="$(JENKINS_CLI_READ_ONLY=1 run "$@" 2>&1 >/dev/null)"; code=$?
  set -e
  if [[ "$code" -ne 5 ]] || ! grep -q '"code": "READONLY_BLOCKED"' <<<"$err"; then
    echo "FAIL - read-only blocks $label (exit $code)"; echo "$err" | head -5; exit 1
  fi
  echo "ok   - read-only blocks $label (exit 5)"
  pass=$((pass + 1))
}
ro_blocks "job build"    job build app --param BRANCH=main
ro_blocks "build stop"   build stop app 7
ro_blocks "queue cancel" queue cancel 77
check "read-only + --dry-run still previews" '"dry_run": true' -- env JENKINS_CLI_READ_ONLY=1 "$BIN" --config "$TMP" build stop app 7 --dry-run
if [[ -s "$WRITE_LOG" ]]; then
  echo "FAIL - read-only sent no write request (server saw: $(tr '\n' ';' <"$WRITE_LOG"))"; exit 1
fi
echo "ok   - read-only sent no write request"
pass=$((pass + 1))

# --allow-writes overrides read-only and actually triggers (mock returns a queue ref).
check "allow-writes triggers" '"triggered": true' -- env JENKINS_CLI_READ_ONLY=1 "$BIN" --config "$TMP" --allow-writes job build app
if grep -qx "POST /job/app/build" "$WRITE_LOG"; then
  echo "ok   - write log records --allow-writes requests"
  pass=$((pass + 1))
else
  echo "FAIL - write log records --allow-writes requests"; exit 1
fi

# A permalink is resolved to its build number before the stop, so the build
# stopped and the one named in the output are the same even if a newer starts.
check "build stop resolves last" '"build": "7"' -- run build stop app last
if grep -qx "POST /job/app/7/stop" "$WRITE_LOG"; then
  echo "ok   - stop on a permalink posts to the build number"
  pass=$((pass + 1))
else
  echo "FAIL - stop on a permalink posts to the build number (server saw: $(tr '\n' ';' <"$WRITE_LOG"))"; exit 1
fi

# Jenkins acknowledges a cancel with a redirect to the queue page (the mock
# answers a stop and a cancel like Jenkins): that redirect is the success.
check "queue cancel accepts Jenkins' redirect" '"cancelled": true' -- run queue cancel 77

# Any other redirect of a write (here a single sign-on proxy's login page)
# says nothing about it: the outcome is unknown, with the redirect's status.
sso_err="$(run build stop sso 3 2>&1 >/dev/null || true)"
if [[ "$sso_err" == *"Write outcome is unknown for build 3 of job sso"* &&
      "$sso_err" == *'"http_status": 302'* && "$sso_err" == *"answered it with a redirect"* ]]; then
  echo "ok   - a write redirected to a login page is an unknown outcome"
  pass=$((pass + 1))
else
  echo "FAIL - a write redirected to a login page is an unknown outcome"; echo "$sso_err" | head -12; exit 1
fi

# Read-only mode is the user's setting: the error offers --allow-writes only for
# an authorized write and never tells an agent to unset the switch.
ro_err="$(JENKINS_CLI_READ_ONLY=1 run job build app 2>&1 >/dev/null || true)"
if [[ "$ro_err" == *"Only if the user authorized this write"* && "$ro_err" != *"unset JENKINS_CLI_READ_ONLY"* ]]; then
  echo "ok   - read-only error leaves the switch to the user"
  pass=$((pass + 1))
else
  echo "FAIL - read-only error leaves the switch to the user"; echo "$ro_err" | head -8; exit 1
fi

# A trigger that fails after it was sent may have queued the build: the error
# says the outcome is unknown, is not retryable and names the reads.
write_err="$(run job build unavailable 2>&1 >/dev/null || true)"
if [[ "$write_err" == *"Write outcome is unknown for build of job unavailable"* &&
      "$write_err" == *'"retryable": false'* && "$write_err" == *"jenkins-cli queue list"* ]]; then
  echo "ok   - failed trigger reports an unknown outcome"
  pass=$((pass + 1))
else
  echo "FAIL - failed trigger reports an unknown outcome"; echo "$write_err" | head -8; exit 1
fi

# Jenkins accepted the build but named no queue item: the build was queued, so
# the error says so instead of reporting success with no queue ref.
set +e
noloc_err="$(run job build no-location 2>&1 >/dev/null)"; code=$?
set -e
if [[ "$code" -ne 0 && "$noloc_err" == *'"code": "WRITE_SUCCEEDED_RESPONSE_INVALID"'* &&
      "$noloc_err" == *"Write succeeded for build of job no-location"* ]]; then
  echo "ok   - trigger without a queue location is reported"
  pass=$((pass + 1))
else
  echo "FAIL - trigger without a queue location is reported (exit $code)"; echo "$noloc_err" | head -8; exit 1
fi

# Exit-code contract: missing required path arg -> usage (2).
set +e
run build get >/dev/null 2>&1; code=$?
set -e
if [[ "$code" -ne 2 ]]; then
  echo "FAIL - missing path arg should exit 2, got $code"; exit 1
fi
echo "ok   - missing-arg exits 2"
pass=$((pass + 1))

# Exercise context persistence without touching the host credential store.
# The interactive edit/add/replace wizard runs in Go tests with MockInit instead.
CFG="$(mktemp -d)"
"$BIN" --config "$CFG" config set-context default --base-url "$URL" --auth-scheme token >/dev/null 2>&1
"$BIN" --config "$CFG" config set-context prod --base-url "$URL" --auth-scheme token >/dev/null 2>"$CFG/err"
if "$BIN" --config "$CFG" config get-contexts 2>/dev/null | grep -q '"name": "prod"'; then
  echo "ok   - credential-free setup adds a second context"
  pass=$((pass + 1))
else
  echo "FAIL - config set-context add-context flow"; cat "$CFG/err" | head -5; exit 1
fi
# The earlier name still lists; delete-context removes the added context, and
# the only context left cannot be deleted.
if "$BIN" --config "$CFG" config contexts 2>/dev/null | grep -q '"name": "prod"' \
   && "$BIN" --config "$CFG" config delete-context PROD 2>/dev/null | grep -q '"status": "deleted"' \
   && ! "$BIN" --config "$CFG" config get-contexts 2>/dev/null | grep -q '"name": "prod"'; then
  set +e; "$BIN" --config "$CFG" config delete-context default >/dev/null 2>"$CFG/err"; code=$?; set -e
  if [[ $code -eq 2 ]] && grep -q '"code": "LAST_CONTEXT"' "$CFG/err"; then
    echo "ok   - config contexts still lists; delete-context removes a context, not the last"
    pass=$((pass + 1))
  else
    echo "FAIL - delete-context of the only context (exit $code)"; head -5 "$CFG/err"; exit 1
  fi
else
  echo "FAIL - config contexts / delete-context"; exit 1
fi
rm -rf "$CFG"

echo ""
echo "e2e: $pass checks passed"

"$ROOT/scripts/e2e-setup.sh"
