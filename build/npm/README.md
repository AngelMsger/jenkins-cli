# @angelmsger/jenkins-cli

npm distribution of [`jenkins-cli`](https://github.com/AngelMsger/jenkins-cli)
— a command-line tool that inspects [Jenkins](https://www.jenkins.io/) jobs,
builds, Pipeline stages and test results from the terminal — and can trigger
builds — built for coding agents (Claude Code and others) and humans alike.

```bash
npm install -g @angelmsger/jenkins-cli
jenkins-cli config init --pretty             # interactive TUI: server URL + credentials
jenkins-cli skill install                    # deploy the companion agent Skill
jenkins-cli build get my-app lastFailed      # why is the latest build red?
```

Installing this package downloads the prebuilt binary for your platform from the
matching GitHub Release and verifies its SHA-256 checksum. If your npm setup
disables install scripts, the binary is fetched on first run instead.

The companion `jenkins` Skill for coding agents is embedded in the binary;
`jenkins-cli skill install` deploys a copy that always matches the installed
CLI version.

See the [project README](https://github.com/AngelMsger/jenkins-cli) and the
[installation guide](https://github.com/AngelMsger/jenkins-cli/blob/main/docs/installation.md)
for full documentation.
