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
`cliflags`, `constants` — are used across the layers. `output`, `cliflags`,
`update`, the credential store, credential resolution, the configuration
engine, and the code behind `pkg/errors` and `pkg/transport` come from
[agent-cli-kit](https://github.com/AngelMsger/agent-cli-kit), shared by every
CLI in the family; jenkins-cli supplies its own hints, User-Agent, keychain
service name, auth schemes, settings table and release repository.

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
finishes, with SIGINT/SIGTERM handling. Its timeout is per request; agents use
a host-side deadline and pin the numeric build before following or resuming.
The three write commands (`job build`,
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
  404 there means CSRF is disabled and the POST proceeds crumbless. Only a 2xx
  acknowledges a write, except Jenkins' own acknowledgment of a stop or cancel
  (below). The kit's transport answers a write with its first response, so
  `postForm` sees a write's redirect, whatever HTTP client a library caller
  passes: the default one never follows it, and when the caller's own client
  follows it, the transport still returns the redirect it began with. Reads
  (`getJSON`, `getText`) also take only a 2xx as the answer, and every 3xx
  that comes back (a write's, a 304, or a read's tenth hop) is built with the
  kit's redirect rule (`CLIError.WithRedirect`): not retryable, with a hint
  naming where it points (scheme and host) and `--base-url`; the write rule
  then puts its own hint on a write. `doctor` reports a redirected check as
  `redirected` (agent-cli-kit's `doctor` package).
- Writes follow the family's write-outcome rule from agent-cli-kit
  (`errors.FromWrite`): each goes through `postWrite` with a write target
  (`write_targets.go`) naming what it changes and the bounded reads that show
  it (`queue list` plus `build list <path> --limit 5` for a trigger, `build get`
  for a stop, `queue get` for a cancel). A definite refusal (usage, auth,
  permission, not found, conflict) is returned unchanged; any other failure,
  such as a timeout or a 5xx after the POST, becomes "Write outcome is unknown
  for …" with `retryable=false`, so an agent checks before triggering a second
  build. A trigger whose response has no queue item `Location` (a URL whose
  path ends in `/queue/item/<n>/`) reports `WRITE_SUCCEEDED_RESPONSE_INVALID`:
  Jenkins queued the build, but the CLI cannot say which one. Jenkins answers a
  trigger itself, so a redirect of a trigger (often to a login page, a single
  sign-on proxy, or `http` to `https`) is an unknown outcome with the
  redirect's status. A stop or cancel is acknowledged by Stapler's
  `forwardToPreviousPage`: with no `Referer`, a 302 to "." of the request, the
  build page for `/job/<path>/<n>/stop` and `/queue/` for `/queue/cancelItem`.
  `forwardedToPreviousPage` accepts a 302 or 303 whose `Location` names the
  request's own host and that directory, with no query and no fragment (a
  fragment-routed login page can sit at the same path). It ignores the scheme
  and port, which the servlet container fills in from its own view (often
  `http` and the container's port behind a TLS proxy). Any other redirect,
  including a 307 or 308, one to another host at the same path (a single
  sign-on proxy), and one to the container's internal host name, is an
  unknown outcome: a redirect that only looks like the acknowledgment never
  reports a stop or a cancel that may not have reached Jenkins.
- `build stop` resolves a permalink (`last`, `lastFailed`, …) to its build
  number with a read before it posts the stop, so the stop and the `build get`
  that reconciles an uncertain stop name the same build even when a newer one
  starts meanwhile. `Client.StopBuild` does this itself and keeps its released
  `error`-only signature; the CLI calls `apiclient.StopBuildNumber`, which
  also returns the number for the output. Given another `Client` (a library
  caller's fake or decorator), `StopBuildNumber` resolves with `GetBuild`,
  stops by number and passes the stop's error through `errors.FromWrite`, so
  a retryable failure never invites a second stop. `pkg/apiclient/compat_test.go` pins
  the `Reader` and `Writer` method sets, so a signature change fails to
  compile. The `--dry-run` preview is offline, so it keeps the permalink in its
  URL.
- `factory.go`'s `BuildClient` normalizes the base URL and constructs the
  transport with the auth decorator. `readonly.go` is the read-only wrapper (see
  [read-only-mode.md](read-only-mode.md)).

Read methods shape their responses with Jenkins' `?tree=` parameter so the
default output stays a compact, high-signal map rather than a full object dump.
`GetBuild` adds the commit the job built (`revision`) from the Git plugin's
BuildData actions, one per repository checked out. With several (a Pipeline
shared library, extra checkouts) it reads the job's own Git remote in one more
request and takes the last checkout of it; when the job declares no remote
(only freestyle and multibranch jobs do) or none match, `revision` is left out
rather than guessed.
Queue items preserve the job `url` and add `cancelled` plus an optional
`executable` using the existing `BuildRef` number/URL shape. Trigger-to-build
handoff follows that executable, never the moving latest-build selector.
Missing stage/test endpoints retain their existing error codes, describe
unavailable evidence without inferring job type or test outcomes, and keep the
requested job/build in their recovery steps.

## Transport (`pkg/transport`)

A re-export of agent-cli-kit's `transport` whose `New` defaults the User-Agent
to `jenkins-cli/<version>`. A thin `Client` that applies request decorators (auth, user-agent) and retries
transient failures. Retries are limited to idempotent methods (GET/HEAD); a
`Retry-After` header on 429/503 takes precedence over linear backoff. `Doer` is
an interface so tests inject fakes.

## Error model (`pkg/errors`)

A re-export of agent-cli-kit's `errors`; `hints.go` holds jenkins-cli's default
guidance and is installed with `SetGuidance` when the package loads. Every
failure is a `*CLIError` with `Category`, a stable `Code`, `Message`,
`Hint`, `NextSteps`, `Retryable`, `HTTPStatus` and an optional structured
`Recovery`. The category drives two
deterministic mappings: the **exit code** (0–11) and the **default
guidance** (`hints.go`). `FromHTTPStatus` classifies HTTP statuses. This is the
"errors as navigation" contract — every failure tells an agent the next command
to run. `Recovery` describes an environment change, such as retrying the same
command in host scope; it is deliberately separate from `Retryable`, which
means a retry in the current environment may work. `doctor` exposes the same
distinction through per-check `status` and optional `recovery_scope`.

## Output (agent-cli-kit `output`)

`Emit` and `EmitList` render any value as `json` (default), `table`, or `ndjson`.
JSON lists use the `{items, next, has_more}` envelope. NDJSON writes only rows
to stdout. For paginated callers the shared renderer emits a structured
`_notice.pagination` record containing `next` and `has_more` to stderr after
successful row output, including filtered empty pages. `Options.NoticeWriter`
defaults to stderr; `Options.NextFlag` controls the notice's `next_steps` and
table footer and defaults to `--cursor`. Notice write failures do not fail a
successful result. Current Jenkins list endpoints are unpaginated and emit no
continuation notice; the renderer's default does not introduce a CLI flag.
`--fields` projects
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
`JENKINS_CONTEXT` override per invocation. `internal/config` registers the
settings (keys, `JENKINS_*` variables, defaults), the file location and the
command wording with agent-cli-kit's `config` package, which owns the layering,
context selection, the atomic file write and set-context planning. jenkins-cli
keeps the YAML shape structs that define the file format.

## Auth (`internal/auth`)

Jenkins authenticates API requests with HTTP Basic. Two schemes, identical on the
wire (`Basic base64(username:secret)`): `token` (username + API token,
recommended) and `basic` (username + password). `internal/auth` registers
them with agent-cli-kit's `auth` package, which owns the credential model, the
header encoding and the recovery errors. `Resolve` produces a validated
`Credential` from config + secrets, loading the secret from the keychain when not
supplied via flags/env. The `Store` (agent-cli-kit's `credstore`, under the
`jenkins-cli` keychain service) prefers the OS keychain (`go-keyring`) and
falls back to a per-user DPAPI-encrypted file on Windows or a `0600` JSON file
on macOS/Linux. On macOS it first checks for a default keychain, so a sandbox
without one never raises the "Reset To Defaults" dialog. A credential becomes a `transport.Decorator`
that authenticates every request. Store access errors are preserved rather than
collapsed into "missing": `CREDENTIAL_STORE_INACCESSIBLE` and
`CREDENTIAL_NOT_VISIBLE_OR_MISSING` carry a host-scope recovery instruction so
an Agent host can retry before asking the user to reconfigure credentials.

The store account is `<host>:<scheme>` (`AccountKey`), so contexts on the same
Jenkins share one secret rather than holding a copy each. `config init` saves
under the normalized server URL and never deletes an entry: respelling a
context's URL or moving it to another server leaves every secret another
context still resolves, at the cost of a harmless orphan when the old server
is no longer configured. `auth logout` removes the shared entry for the
active context's server and scheme. `config delete-context` removes the
deleted context's entry only when no remaining context resolves the same
account, and only after the config file is written.

## Time (`pkg/timeutil`)

`FromMillis` converts Jenkins' epoch-millisecond timestamps to UTC; `HumanSince`
renders a compact relative phrase (`3h ago`). Builds carry both an ISO
`started_at` and a relative `started_ago`, so an agent never hand-computes
elapsed time.

## Skill embedding (`assets.go`)

The companion Skill is embedded with `//go:embed all:skills/jenkins`, so a binary
always ships a Skill matching its version. A test (`assets_test.go`) guards the
Skill `description` against Codex's 1024-character limit.

`skill install` uses the agent path table in agent-cli-kit's
`skill` package, mapping each agent to its global / project skills
directory and probe markers: Claude Code uses `~/.claude/skills` and `./.claude/skills`; Codex uses `~/.codex/skills` and `./.agents/skills`; Cursor uses `~/.cursor/skills` and `./.cursor/skills`; the shared Agents tree uses `~/.agents/skills` and `./.agents/skills`; Gemini CLI uses `~/.gemini/skills` and `./.gemini/skills`; GitHub Copilot uses `~/.copilot/skills` and `./.agents/skills`; OpenCode uses `~/.config/opencode/skills` and `./.opencode/skills`; Continue uses `~/.continue/skills` and `./.continue/skills`; Windsurf uses `~/.codeium/windsurf/skills` and `./.windsurf/skills`; Grok Build uses `~/.grok/skills` and `./.grok/skills`; Pi uses `~/.pi/agent/skills` and `./.pi/skills`; Kilo Code uses `~/.kilocode/skills` and `./.kilocode/skills`; Roo Code uses `~/.roo/skills` and `./.roo/skills`. With no flag it
probes which directories exist and installs / removes for each hit;
`--agent` selects explicitly; `--dir` is the agent-agnostic explicit path.

The Skill handshake carries its version, not a boolean. Runtime hints compare
the loaded version with the embedded copy; `skill status` also reads deployed
`SKILL.md` frontmatter and classifies each installation as current, outdated,
or unknown. Update notices return ordered steps to upgrade the CLI, refresh the
Skill, and reload the agent context. `doctor` reports Skill state as an
informational check that does not change connectivity health.

## Generated reference (`cmd/gen-docs`)

Walks the live cobra tree (`app.NewRootCmd`) and emits `docs/cli/index.html`
(styled, sidebar-grouped, served by Pages) and `docs/cli/README.md` (a
module-grouped table). Because both come from the command tree, they can't drift
from `--help`; CI fails if the committed output is stale.

## Testing

- Unit tests cover `timeutil`, the error mappings, output projection, and the API
  client against an `httptest` server (the job/build/console/stages/tests
  decoders, the dry-run plan builders). The read-only tests check their tables
  against the `Writer` and `Reader` method sets: every write must return
  `READONLY_BLOCKED` naming its operation without sending a request, and every
  read must reach the inner client with the same arguments and return its
  results unchanged.
- `scripts/e2e.sh` runs the built binary against `test/mockserver` and asserts
  the agent-facing contract (JSON output, structured errors, exit codes) with no
  real credentials. Every write command runs under read-only mode, and the mock
  server's write log (`MOCK_WRITE_LOG`) must stay empty until the first
  `--allow-writes` call. Queue lifecycle tests cover waiting, assigned and cancelled
  items; the embedded shell error-handling example is executed against stubbed
  exit codes so its recovery behavior stays correct.

## Team service setup and login persistence

`internal/config/setup.go` owns the acquisition links and wraps the shared
offline service-field validation and the pure `PlanServiceContext` merge used by
execution and dry-run.
`LoadOptions.Setup` selects the requested destination even when it is new and
filters personal environment fields before credential-based scheme inference.
Ordinary runtime precedence and transient environment secrets remain supported.
`AuthConfig.CredentialURL` is additive, non-secret, and serialized through every
config shape.

`internal/app/setup.go` wires `config set-context`, `auth guide`, and wizard
prefill. `internal/app/auth_login.go` checks the complete normalized service URL,
verifies authentication, stores the secret, and persists the associated identity.
Expected storage failures preserve their causes and distinguish a stored secret
from a completed login. Error payloads may include optional non-secret `details`;
context conflicts use field-level `before`/`after` values. Setup does not access a
credential store and is available under the remote read-only posture.

The end-to-end setup harness (`scripts/e2e-setup.sh`, part of `make e2e`) runs
without a service or user credentials. Unit tests cover target-specific
precedence, persistence failures, guide URLs, and reloading a stored login.
See [installation](installation.md#team-distribution-and-personal-login) for the
canonical user-facing contract and product-specific authentication guidance.

### Existing credential reuse

The decision is agent-cli-kit's `reuse` package, shared by the family.
`internal/app/auth_reuse.go` describes the stored contexts to it
(`config.ReuseContexts`: service identity = normalized URL, credential-key
spelling, effective scheme, username), and supplies native resolution
(`auth.ResolveLookup`, which also reports what the keychain and file answered),
verification bounded by the reuse deadline (`verifyCredentialWithin`, which
returns the `/me/api/json` user so the account costs no extra request: `id` is
the canonical account, `fullName` its display name) and the write, which
rereads and compares the config before replacing it. Both registered schemes are
HTTP Basic, so the kit treats a username under either as an identity. It never
writes the credential store, consumes environment secrets or changes
current_context. Public `config set-context` remains offline and credential-free.
`config.Load` computes the runtime-only `Config.ReuseStep` offline, which the
guide, the setup plan, credential errors, `doctor` and HTTP 401 errors (in
`root.Execute`) list before sign-in. See the installation guide for the result and
recovery contract.

Equivalent service URL overrides preserve the persisted native credential lookup
key without redirecting requests or copying secrets. Logout removes that same
entry. A different complete deployment URL cannot use the retained lookup key.
