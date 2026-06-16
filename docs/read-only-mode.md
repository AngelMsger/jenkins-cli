# Read-only mode

Read-only mode is a session-level safety switch that blocks every mutating
client method **before any HTTP request is sent**. It gives you a "let an agent
explore freely without risk" posture.

## What it gates

`jenkins-cli` is read-first: discovery and inspection (`job`, `build`, `queue`
reads) are always allowed. Three commands mutate Jenkins and are gated by
read-only mode:

- `job build` — trigger a build
- `build stop` — abort a running build
- `queue cancel` — drop a queued build

Jenkins' control plane (creating / configuring jobs, credentials, nodes) is on
the roadmap; each new write method plugs into this same wrapper without rework.

## Enabling it

Three layers, in precedence order:

1. **Config file** — `defaults.read_only: true` in
   `~/.angelmsger/jenkins/config.yaml`.
2. **Environment** — `JENKINS_CLI_READ_ONLY=1`.
3. **Per-invocation override** — the root `--allow-writes` flag flips the posture
   back to read-write for a single command:

   ```bash
   JENKINS_CLI_READ_ONLY=1 jenkins-cli --allow-writes job build my-app
   ```

`appState.readOnly()` is `defaults.read_only && !--allow-writes`.

## How it works

When the posture is read-only, the API client is wrapped by
`apiclient.NewReadOnly` before any command runs (`internal/apiclient/readonly.go`).
The wrapper embeds the real client, so every **read** passes straight through.
Each **write** method (`TriggerBuild`, `StopBuild`, `CancelQueueItem`) overrides
the embedded one to return a structured error instead of issuing a request:

```json
{
  "error": {
    "category": "permission",
    "code": "READONLY_BLOCKED",
    "message": "refusing to trigger a build: read-only mode is enabled",
    "hint": "This session is read-only. Preview the request with --dry-run, or re-run with --allow-writes to override.",
    "next_steps": ["Add --dry-run to preview the request without sending it", "Add --allow-writes to override read-only mode for this command"],
    "retryable": false
  }
}
```

`READONLY_BLOCKED` maps to exit code 5 (`permission`). The block happens in the
client, so it is enforced regardless of which command triggered the write.

## Relationship to `--dry-run`

`--dry-run` prints the HTTP request that *would* be sent without sending it. It
is built from a pure `apiclient.*Plan` function that touches no network and never
calls a mutating client method — so it remains usable under read-only mode. Thus
`--dry-run` answers "what would this do?" and read-only answers "make sure
nothing can actually do it"; they compose.
