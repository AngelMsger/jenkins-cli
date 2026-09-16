# Errors and exit codes

Every failure is JSON on **stderr** (stdout stays clean), shaped:

```json
{
  "error": {
    "category": "not_found",
    "code": "HTTP_NOT_FOUND",
    "message": "Jenkins returned HTTP 404",
    "hint": "No such job or build at that path. Job paths are folder/job[/branch]; list to find the exact name.",
    "next_steps": ["jenkins-cli job list", "jenkins-cli job get <path>"],
    "http_status": 404
  }
}
```

Read `next_steps` first — it names the command to run next. `retryable` (when
present) tells you whether a retry in the same environment could help
(rate-limit / network / server). Environment changes use the optional
`recovery` object instead.

## Category → exit code

| Category     | Exit | Meaning / typical fix |
|--------------|------|-----------------------|
| (success)    | 0    | — |
| `internal`   | 1    | Unexpected bug; retry with `--verbose`. |
| `usage`      | 2    | Bad flag/argument; check `--help`. Bad `--param`, non-numeric queue id, etc. |
| `config`     | 3    | Config/credential resolution failed; inspect `code` and `recovery` before reconfiguring. |
| `auth`       | 4    | Credentials rejected → `auth status`, re-run `config init`. |
| `permission` | 5    | Authenticated but not allowed, or read-only mode blocked a write. |
| `not_found`  | 6    | Unknown job / build → `job list`, `build list <path>`. |
| `rate_limit` | 7    | Too many requests; retryable. |
| `network`    | 8    | Server unreachable; retryable → `doctor`, check `--base-url`. |
| `server`     | 9    | Jenkins 5xx; retryable. |
| `parse`      | 10   | Response didn't match expectations; retry with `--verbose`. |
| `conflict`   | 11   | Resource changed since read; re-fetch then retry. |

Scripted use:

```bash
build_file=$(mktemp)
trap 'rm -f "$build_file"' EXIT
if jenkins-cli build get my-app lastBuild >"$build_file"; then
  cat "$build_file"
else
  code=$?
  case "$code" in
    3|4) printf '%s\n' 'Inspect the structured auth/config recovery steps.' >&2 ;;
    6) printf '%s\n' 'Verify the selected job and build.' >&2 ;;
    7|8|9) printf '%s\n' 'Transient read failure; retry within a bounded budget.' >&2 ;;
    *) printf 'Command failed with exit %s.\n' "$code" >&2 ;;
  esac
  exit "$code"
fi
```

## Common cases

- **`CREDENTIAL_STORE_INACCESSIBLE` / `CREDENTIAL_NOT_VISIBLE_OR_MISSING`
  (config/3)** — when `recovery.scope` is `host`, request host access and retry
  the same invocation once. Repeating it in the same sandbox will not help.
  Only configure credentials when the host retry also reports them missing.
- **`NO_BASE_URL` (config/3)** — no server configured. Run `config init` or set
  `JENKINS_URL`.
- **`AUTH_LOGIN_NEEDS_TTY` (auth/4)** — `auth login` needs a terminal; plain
  `config init` also supports piped input. In CI / agents set `JENKINS_USER` +
  `JENKINS_TOKEN` (or `JENKINS_PASSWORD`).
- **`HTTP_UNAUTHORIZED` (auth/4)** — Jenkins rejected the credentials. Use your
  Jenkins **username + API token** (User → Configure → API Token), not your web
  password. Re-run `config init`.
- **`HTTP_FORBIDDEN` (permission/5)** — authenticated but the user lacks
  permission for the resource (needs Job/Read, or Job/Build for a trigger), or a
  required CSRF crumb was missing. Check the user's permissions in Jenkins →
  Manage Jenkins → Security.
- **`HTTP_NOT_FOUND` (not_found/6)** — no job or build at that path. Paths are
  `folder/job[/branch]` and case-sensitive; run `job list` / `build list`.
- **`NOT_PIPELINE` (not_found/6)** — the stages endpoint returned 404. Verify
  the same numeric build exists and inspect `job get` for the job kind before
  deciding stage reporting is unavailable. Use that build's console next.
- **`NO_TEST_REPORT` (not_found/6)** — the test-report endpoint returned 404.
  Verify the same build and read its console; this does not prove tests ran
  or passed.
- **`READONLY_BLOCKED` (permission/5)** — a write (`job build`, `build stop`,
  `queue cancel`) was blocked because the session is read-only. Preview with
  `--dry-run`; use `--allow-writes` only for a write the user authorized.
- **`BAD_PARAM` (usage/2)** — `--param` must be `KEY=VALUE`.
