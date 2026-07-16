# Getting started: configuration and auth

`jenkins-cli` talks to any Jenkins instance (self-hosted or hosted), default
`http://localhost:8080`. Authentication is HTTP Basic with your Jenkins
**username + API token** (recommended) or username + password.

## Get an API token

In Jenkins: click your name (top right) → **Configure** → **API Token** → **Add
new Token** → generate and copy it. An API token works even when the UI login
goes through SSO, and can be revoked independently of your password.

## Interactive setup (humans, on a terminal)

```
jenkins-cli config init --pretty   # interactive TUI (recommended for humans)
jenkins-cli config init             # plain line-by-line wizard (works over a pipe)
```

Prompts for the server URL, username, auth scheme and secret, verifies them
against the server, then writes a context to
`~/.angelmsger/jenkins/config.yaml` and stores the secret in the OS keychain
(falling back to per-user DPAPI on Windows or a `0600` file on macOS/Linux).
When a config
already exists, re-running it lists the contexts and asks whether to **edit** one
(prefilled), **add** a new one, or **replace** everything; `config init --context
prod` skips that prompt and targets the named context directly.

`--pretty` renders an interactive TUI and requires a terminal (otherwise it
fails with `PRETTY_NEEDS_TTY`); the plain wizard reads stdin line by line, so it
also works non-interactively for scripted setup.

`auth login` is the lighter form: it re-prompts only for credentials of the
already-configured context.

## For agents and sandboxes

The user has normally configured Jenkins already. Reuse their host config under
`~/.angelmsger/jenkins/` and OS keychain. If credential resolution returns
`CREDENTIAL_STORE_INACCESSIBLE` or `CREDENTIAL_NOT_VISIBLE_OR_MISSING` with
`recovery.scope=host`, request host access and retry the same invocation once.
Do not run `config init` or `auth login` inside the sandbox.

Only when the host retry also reports missing credentials should the user
configure them in their terminal or provide environment variables. The CLI
cannot and must not elevate itself; `recovery.scope=host` is an instruction to
the Agent host or approval layer.

## Headless setup (CI and genuinely unconfigured environments)

Provide everything via environment variables; they take precedence over the
config file and keychain:

```
export JENKINS_URL=https://jenkins.example.com
export JENKINS_USER=alice
export JENKINS_TOKEN='11abc…your-api-token'
# …or a password instead of an API token:
export JENKINS_PASSWORD='your-password'
```

Both schemes go on the wire as HTTP Basic (`username:secret`); `JENKINS_TOKEN`
selects the token scheme, `JENKINS_PASSWORD` the basic scheme.

## Verify

```
jenkins-cli auth status     # identity (whoami) + reachability
jenkins-cli doctor          # config / credentials / connectivity checks
```

## Precedence

Highest wins: command-line flags → environment variables → `.env` file → config
file → built-in defaults. `config show` prints the resolved values and where the
key ones came from.

## Read-only posture

Set `JENKINS_CLI_READ_ONLY=1` (or `defaults.read_only: true` in the config file)
to assert a read-only session: the write commands (`job build`, `build stop`,
`queue cancel`) are then blocked before any request is sent. `--allow-writes` is
the per-call escape hatch, and `--dry-run` previews a write without sending it
even in read-only mode.
