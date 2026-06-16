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
(`jenkins-cli-<os>-<arch>`), `chmod +x` it, and put it on your `PATH`.

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
```

Run `jenkins-cli completion --help` for persistent-install instructions per
shell.

## 3. Install the companion Skill

The `jenkins` Skill is embedded in the binary, so it always matches the CLI
version. `skill install` detects your coding agents (Claude Code, Codex) and
installs into each:

```bash
jenkins-cli skill install                 # auto-detect, install for each agent
jenkins-cli skill install --agent codex   # target one agent
jenkins-cli skill install --project       # into ./.claude/skills, ./.agents/skills
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

Then verify:

```bash
jenkins-cli doctor       # config / credentials / connectivity
jenkins-cli auth status  # identity + reachability
```

Configuration resolves in precedence order (highest first): CLI flags →
environment (`JENKINS_*`) → `.env` → `~/.angelmsger/jenkins/config.yaml`
→ defaults. Secrets are stored in the OS keychain (with a `0600` file fallback)
and never written to the config file. See `.env.example` for the full variable
list, and the companion Skill's
[getting-started reference](../skills/jenkins/references/getting-started.md)
for auth details, including SSO / Service Accounts.
