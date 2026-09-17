# Installation & setup

## 1. Install the CLI

### npm (recommended)

```bash
npm install -g @angelmsger/jenkins-cli
```

The package's `postinstall` downloads the prebuilt binary for your platform from
the matching GitHub Release, verifies its SHA-256 checksum, and installs it.
Upgrade with `npm update -g @angelmsger/jenkins-cli`. Installs done with
`--ignore-scripts` fetch the binary lazily on first run.

### go install

```bash
go install github.com/angelmsger/jenkins-cli/cmd/jenkins-cli@latest   # Go 1.24+
```

### From source

```bash
git clone https://github.com/AngelMsger/jenkins-cli
cd jenkins-cli
make install          # builds and copies to $GOBIN (or $GOPATH/bin)
```

### Prebuilt binary

Download the asset for your platform from the
[Releases page](https://github.com/AngelMsger/jenkins-cli/releases)
(`jenkins-cli-<os>-<arch>`), verify it against `checksums.txt`, and put it on
your `PATH`.

On macOS/Linux, run `chmod +x jenkins-cli-*` before moving the binary. On
Windows PowerShell, download `jenkins-cli-windows-amd64.exe` (or
`windows-arm64.exe`) together with `checksums.txt`, then:

```powershell
$asset = "jenkins-cli-windows-amd64.exe"
$checksumLine = Get-Content .\checksums.txt | Where-Object { $_ -match "\s+$([regex]::Escape($asset))$" } | Select-Object -First 1
if (-not $checksumLine) { throw "No checksum found for $asset" }
$expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash ".\$asset" -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "SHA-256 mismatch for $asset" }
$binDir = Join-Path $HOME "bin"
New-Item -ItemType Directory -Force $binDir | Out-Null
Move-Item ".\$asset" (Join-Path $binDir "jenkins-cli.exe")
[Environment]::SetEnvironmentVariable("Path", ([Environment]::GetEnvironmentVariable("Path", "User") + ";$binDir"), "User")
```

Open a new PowerShell window after changing `PATH`.

## 2. Enable shell completion (optional)

`jenkins-cli` completes subcommands and enum flag values.

```bash
# bash — current shell
source <(jenkins-cli completion bash)

# zsh — persistent
jenkins-cli completion zsh > "${fpath[1]}/_jenkins-cli"

# fish
jenkins-cli completion fish | source

# PowerShell
jenkins-cli completion powershell | Out-String | Invoke-Expression

# PowerShell — persistent
jenkins-cli completion powershell >> $PROFILE
```

Run `jenkins-cli completion --help` for persistent-install instructions per
shell.

## 3. Install the companion Skill

The `jenkins` Skill is embedded in the binary, so it always matches the CLI
version. `skill install` detects your coding agents — **Claude Code**, **Codex**,
**Cursor**, **Agents** (shared), **Gemini CLI**, **GitHub Copilot**,
**OpenCode**, **Continue**, **Windsurf**, **Grok Build**, **Pi**,
**Kilo Code**, and **Roo Code** — and installs into each:

```bash
jenkins-cli skill install                 # auto-detect, install for each agent
jenkins-cli skill install --agent codex   # target one agent
jenkins-cli skill install --project       # into each agent's project skills dir
jenkins-cli skill uninstall               # remove it
jenkins-cli skill path                     # show where it would install, and status
```

`skill path` prints every agent's resolved location and install status, so use
it rather than memorising the per-agent directories.

After every CLI upgrade, run `jenkins-cli skill install`, then reload the
agent context. `jenkins-cli skill status` compares the loaded, installed, and
embedded versions and reports the next steps when they differ.

Alternatively, install it from the git repository with the `npx skills` workflow:

```bash
npx skills add AngelMsger/jenkins-cli
```

## 4. Configure

Set up a server interactively, or via environment variables for headless use:

```bash
jenkins-cli config init --pretty   # interactive TUI (recommended for humans)
jenkins-cli config init             # plain wizard (works over a pipe / scripts)
```

```bash
export JENKINS_URL=https://jenkins.example.com
export JENKINS_USER=alice
export JENKINS_TOKEN='11abc…your-api-token'
# or, for password (basic) auth: export JENKINS_PASSWORD='your-password'
```

PowerShell uses `$env:` for the same headless setup:

```powershell
$env:JENKINS_URL = "https://jenkins.example.com"
$env:JENKINS_USER = "alice"
$env:JENKINS_TOKEN = "<api-token>"
jenkins-cli doctor
```

Then verify:

```bash
jenkins-cli doctor       # config / credentials / connectivity
jenkins-cli auth status  # identity + reachability
```

Configuration resolves in precedence order (highest first): CLI flags →
environment (`JENKINS_*`) → `.env` → `~/.angelmsger/jenkins/config.yaml`
→ defaults. Secrets are stored in the OS keychain. If Windows Credential
Manager is unavailable, the fallback file is encrypted with per-user DPAPI;
macOS/Linux retain the `0600` fallback. Secrets are never written to the config
file. See `.env.example` for the full variable list, and the companion Skill's
[getting-started reference](../skills/jenkins/references/getting-started.md)
for auth details, including SSO / Service Accounts.

## Team distribution and personal login

Distribute service settings separately from each member's credentials. An installer
can write a named context without a network connection or access to the keychain:

```bash
jenkins-cli config set-context team \
  --base-url https://service.example.com/deploy \
  --auth-scheme token --activate

# The member completes personal authentication in a terminal.
jenkins-cli --use-context team auth guide
jenkins-cli --use-context team auth login
```

`config set-context <name>` resolves **flags > environment > `.env` > the named
target context > defaults**. It ignores personal environment fields and secrets,
including secret-based scheme inference. It never verifies connectivity, reads or
writes the keychain, or changes another context's values or shared defaults.
Existing usernames remain unchanged. The first context becomes current;
subsequent calls change the current context only with `--activate`.

Identical presets do not rewrite the file. Conflicting non-empty service fields
return `CONFIG_CONTEXT_CONFLICT` with a `details` object containing each field's
`before` and `after` values. Inspect those differences, then use `--overwrite`
to update the supplied service fields, or use another context name. Unspecified
fields are retained. `--dry-run` uses the same merge and conflict checks and
returns the proposed changes without writing anything; use `--overwrite
--dry-run` to preview a deliberate conflicting update.

A launcher or CI environment can instead inject service settings on every run:

```bash
export JENKINS_URL=https://service.example.com/deploy
export JENKINS_AUTH_SCHEME=token
# Optional: a verified page for the team's version or an internal setup guide.
export JENKINS_CREDENTIAL_URL=https://help.example.com/jenkins/credentials
jenkins-cli auth guide
jenkins-cli auth login
```

Exports must be sourced into the member's shell or injected by a launcher/CI;
an executed child script cannot export values back into its parent shell.
`--auth-scheme` and `--credential-url` override these variables. The optional
page is persisted as `auth.credential_url` by `config set-context` and is
**display-only**: the CLI never sends an API request or credential to it.
Service paths such as `/deploy` are retained when deriving page links.

`auth guide` works offline and emits `server`, `flavor` where applicable,
`scheme`, `credential_url`, `source`, `instructions`, `documentation_url`, and
`next_steps`. Sources are `flag`, `env`, `dotenv`, `file`, `builtin`, or `fallback`.
There is no server-version probe; navigation instructions accompany version-
dependent links.

Token guidance uses `<base-url>/me/security`. Older releases may use the user Configure
→ API Token page; a team can override the link. Both token and basic schemes require the
Jenkins username. Verification checks the authenticated user, rather than treating an
anonymously readable instance page as successful authentication.

`auth login` reuses the resolved service, shows the same guide on stderr, asks
only for the missing username and the secret, and verifies authentication before
saving. It saves the username/scheme in the config and the secret in the existing
secure store; an environment-only service becomes a default context if none
exists. A later process can resolve that identity without another username
prompt. A different full service URL (including deployment path) in the selected
context fails with `CONTEXT_BASE_URL_MISMATCH` before storing a credential.

`CREDENTIAL_SAVE_FAILED` means validation succeeded but storage failed.
`LOGIN_CONFIG_WRITE_FAILED` means the credential was stored but its config
identity could not be recorded; its details preserve the server/context/scheme
and `credential_stored: true`. Fix file access and run `auth login` again.
Configuration files are replaced atomically. Normal credential environment
variables remain transient and are never copied by `config set-context`.
Non-interactive users supply credentials through the documented environment
variables rather than piping secrets into `auth login`. `config init` retains its
edit/add/replace flow and now uses target-specific presets and the same guide.
