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
version. `skill install` detects your coding agents (Claude Code, Codex, Grok Build, Pi, Pi) and
installs into each:

```bash
jenkins-cli skill install                 # auto-detect, install for each agent
jenkins-cli skill install --agent codex   # target one agent
jenkins-cli skill install --project       # into ./.claude/skills, ./.agents/skills, ./.grok/skills, ./.pi/skills
jenkins-cli skill uninstall               # remove it
jenkins-cli skill path                     # show where it would install, and status
```

Re-run `skill install` after upgrading the CLI to keep the Skill version-matched.

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
