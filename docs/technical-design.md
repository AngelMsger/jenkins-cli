# Technical design

`jenkins-cli` is a Go + [Cobra](https://github.com/spf13/cobra) CLI for a
developer's Jenkins debugging workflow, built to the agent-facing conventions it
shares with its sibling projects (`confluence-cli`, `bitbucket-cli`,
`openobserve-cli`). This document describes the architecture and the `internal/` and `pkg/`
package layout.

## Overview

A command flows through four layers:

```
cmd/jenkins-cli  →  internal/app  →  pkg/apiclient  →  pkg/transport
   (process entry)       (cobra tree,      (Jenkins API       (retrying HTTP,
                          appState,         surface + models)       auth decorator)
                          rendering)
```

- `cmd/jenkins-cli/main.go` is a three-line entry point: `os.Exit(app.Execute())`.
- `internal/app` builds the cobra command tree, resolves configuration and
  credentials, calls the API client, and renders the result.
- `pkg/apiclient` is the typed Jenkins API surface.
- `pkg/transport` is a flavor-agnostic retrying HTTP client.

Cross-cutting packages — `errors`, `output`, `config`, `auth`, `timeutil`,
`cliflags`, `constants` — are used across the layers.

## Command layer (`internal/app`)

`root.go` assembles the tree and owns `Execute()`. Before cobra parses argv,
`cliflags.Normalize` rewrites common LLM slips (camelCase flag names,
flag-stuck-to-value) and echoes each correction as a `_notice` on stderr. On
error, the outermost handler converts the error to a `*errors.CLIError`, writes
it to stderr, and returns the mapped exit code.

`context.go` holds **`appState`**, the runtime context built once in the root
command's `PersistentPreRunE` and captured by every subcommand:

- `load()` resolves configuration from all layers (see Config).
- `newClient()` resolves credentials, builds an authenticated API client, and —
  when the session is read-only — wraps it with `apiclient.NewReadOnly`.
- `emit()` / `emitList()` render results; `timeout()` and `readOnly()` are
  convenience accessors.

Each noun lives in its own file (`job.go`, `build.go`, `queue.go`, `auth.go`,
`config.go`, `doctor.go`, `skill.go`), organised `<noun> <verb>`. `build_log.go`
hosts `build log`, including the `--follow` loop that streams the progressive
console (using the `X-More-Data` / `X-Text-Size` headers) until the build
finishes, with SIGINT/SIGTERM handling. The three write commands (`job build`,
`build stop`, `queue cancel`) build a `--dry-run` preview from a pure
`apiclient.*Plan` function and otherwise call the mutating client method.

## API client (`pkg/apiclient`)

`Client` is an interface (`client.go`); `apiClient` is the single
implementation. One file per resource: `jobs.go`, `builds.go`, `console.go`,
`pipeline.go`, `tests.go`, `changes.go`, `queue.go`, `writes.go`, plus
`instance.go` (ping / whoami).

- `paths.go` is where the CLI absorbs Jenkins' URL footguns: `jobPath` turns a
  human `folder/job[/branch]` path into Jenkins' `/job/.../job/...` form, and
  `buildRef` maps friendly build references (`last`, `lastFailed`, …) to Jenkins
  permalink segments.
- `models.go` defines the curated output types and normalizes Jenkins' quirks: a
  job's `color` → a stable `status`, and a `_class` → a friendly `kind`.
- `getJSON` builds the request, applies the transport, and on a non-2xx response
  calls `httpError`, which classifies the status into a category and attaches
  task-specific guidance (401 → use an API token; 404 → list to find the path).
- `getText` backs the plain-text console endpoints; `postForm` performs writes,
  attaching a CSRF crumb fetched (once, cached) from `/crumbIssuer/api/json` — a
  404 there means CSRF is disabled and the POST proceeds crumbless.
- `factory.go`'s `BuildClient` normalizes the base URL and constructs the
  transport with the auth decorator. `readonly.go` is the read-only wrapper (see
  [read-only-mode.md](read-only-mode.md)).

Read methods shape their responses with Jenkins' `?tree=` parameter so the
default output stays a compact, high-signal map rather than a full object dump.

## Transport (`pkg/transport`)

A thin `Client` that applies request decorators (auth, user-agent) and retries
transient failures. Retries are limited to idempotent methods (GET/HEAD); a
`Retry-After` header on 429/503 takes precedence over linear backoff. `Doer` is
an interface so tests inject fakes.

## Error model (`pkg/errors`)

Every failure is a `*CLIError` with `Category`, a stable `Code`, `Message`,
`Hint`, `NextSteps`, `Retryable`, `HTTPStatus` and an optional structured
`Recovery`. The category drives two
deterministic mappings: the **exit code** (`codes.go`, 0–11) and the **default
guidance** (`hints.go`). `FromHTTPStatus` classifies HTTP statuses. This is the
"errors as navigation" contract — every failure tells an agent the next command
to run. `Recovery` describes an environment change, such as retrying the same
command in host scope; it is deliberately separate from `Retryable`, which
means a retry in the current environment may work. `doctor` exposes the same
distinction through per-check `status` and optional `recovery_scope`.

## Output (`internal/output`)

`Emit` and `EmitList` render any value as `json` (default), `table`, or `ndjson`.
Lists always use the `{items, next, has_more}` envelope. `--fields` projects
results to dot-path keys before rendering. `--pretty` enables ANSI-colored JSON
on a TTY (and is silently downgraded to plain JSON off a TTY). `build log` is the
one command that prints raw text rather than JSON — console output is inherently
text and grep-able.

## Configuration (`internal/config`)

Layered resolution, highest precedence first: **flags → env (`JENKINS_*`) →
`.env` → YAML config file → built-in defaults**. Each field's provenance is
tracked so `config show` can report where a value came from. Secrets (tokens,
passwords) are never written to the YAML file. The file supports multiple
kubectl-style named **contexts** with a `current_context`; `--use-context` and
`JENKINS_CONTEXT` override per invocation.

## Auth (`internal/auth`)

Jenkins authenticates API requests with HTTP Basic. Two schemes, identical on the
wire (`Basic base64(username:secret)`): `token` (username + API token,
recommended) and `basic` (username + password). `Resolve` produces a validated
`Credential` from config + secrets, loading the secret from the keychain when not
supplied via flags/env. The `Store` prefers the OS keychain (`go-keyring`) and
falls back to a per-user DPAPI-encrypted file on Windows or a `0600` JSON file
on macOS/Linux. A credential becomes a `transport.Decorator`
that authenticates every request. Store access errors are preserved rather than
collapsed into "missing": `CREDENTIAL_STORE_INACCESSIBLE` and
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` carry a host-scope recovery instruction so
an Agent host can retry before asking the user to reconfigure credentials.

## Time (`pkg/timeutil`)

`FromMillis` converts Jenkins' epoch-millisecond timestamps to UTC; `HumanSince`
renders a compact relative phrase (`3h ago`). Builds carry both an ISO
`started_at` and a relative `started_ago`, so an agent never hand-computes
elapsed time.

## Skill embedding (`assets.go`)

The companion Skill is embedded with `//go:embed all:skills/jenkins`, so a binary
always ships a Skill matching its version. A test (`assets_test.go`) guards the
Skill `description` against Codex's 1024-character limit.

`skill install` uses an agent path table (`agentSpecs` in
`internal/app/skill.go`) mapping each agent to its global / project skills
directory and probe markers: Claude Code uses `~/.claude/skills` and `./.claude/skills`; Codex uses `~/.codex/skills` and `./.agents/skills`; Cursor uses `~/.cursor/skills` and `./.cursor/skills`; the shared Agents tree uses `~/.agents/skills` and `./.agents/skills`; Gemini CLI uses `~/.gemini/skills` and `./.gemini/skills`; GitHub Copilot uses `~/.copilot/skills` and `./.agents/skills`; OpenCode uses `~/.config/opencode/skills` and `./.opencode/skills`; Continue uses `~/.continue/skills` and `./.continue/skills`; Windsurf uses `~/.codeium/windsurf/skills` and `./.windsurf/skills`; Grok Build uses `~/.grok/skills` and `./.grok/skills`; Pi uses `~/.pi/agent/skills` and `./.pi/skills`; Kilo Code uses `~/.kilocode/skills` and `./.kilocode/skills`; Roo Code uses `~/.roo/skills` and `./.roo/skills`. With no flag it
probes which directories exist and installs / removes for each hit;
`--agent` selects explicitly; `--dir` is the agent-agnostic explicit path.

## Generated reference (`cmd/gen-docs`)

Walks the live cobra tree (`app.NewRootCmd`) and emits `docs/cli/index.html`
(styled, sidebar-grouped, served by Pages) and `docs/cli/README.md` (a
module-grouped table). Because both come from the command tree, they can't drift
from `--help`; CI fails if the committed output is stale.

## Testing

- Unit tests cover `timeutil`, the error mappings, output projection, and the API
  client against an `httptest` server (the job/build/console/stages/tests
  decoders, the dry-run plan builders, and the read-only blocks).
- `scripts/e2e.sh` runs the built binary against `test/mockserver` and asserts
  the agent-facing contract (JSON output, structured errors, exit codes) with no
  real credentials.
