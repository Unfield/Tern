# Self-hosted GitHub Actions runner (Hetzner)

Tern's CI can run trusted jobs on a dedicated Hetzner Cloud server instead of GitHub-hosted runners.

## Security model: trusted jobs only

Tern is a public repository. Anyone can open a pull request that changes a workflow, so **a self-hosted runner must never run pull request code**. The rules:

- The runner only takes jobs from **pushes to `main`** (and, later, tags and releases). Only maintainers can trigger those.
- **All pull requests**, including from forks and Dependabot, run on GitHub-hosted runners.
- The routing lives in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml). Never add `pull_request_target` or any other PR-triggered job that runs on `self-hosted`.
- The server is dedicated to CI. Do not put secrets, databases or other services on it.

## 1. Create the server

In the [Hetzner Cloud Console](https://console.hetzner.cloud):

1. **Image:** Ubuntu 24.04.
2. **Type:** around 4 vCPU and 8 GB RAM is plenty for Tern's CI. x86 matches most users; Arm (CAX) is cheaper and also supported.
3. **SSH key:** add your public key. Do not enable password login.
4. **Firewall:** create a Hetzner Cloud Firewall allowing inbound TCP 22 only, ideally restricted to your own IP. The runner makes outbound connections only, so it needs no other inbound ports.
5. Name it e.g. `tern-ci-1`.

## 2. Install the runner

From your own machine, with the [GitHub CLI](https://cli.github.com) signed in as a repo admin:

```sh
ssh root@SERVER_IP "RUNNER_TOKEN=$(gh api -X POST repos/Unfield/Tern/actions/runners/registration-token --jq .token) bash -s" < infra/github-runner/setup.sh
```

The registration token is generated locally, is only valid for one hour, and can only register a runner. It is never written to disk.

The script:

- installs build tools and enables automatic security updates
- disables SSH password login and turns on the `ufw` firewall and `fail2ban`
- creates an unprivileged `runner` user without sudo
- downloads the latest runner release and **verifies its SHA-256 checksum**
- registers the runner with the label `hetzner` and installs it as a systemd service

Optional variables: `RUNNER_NAME`, `RUNNER_LABELS`, `RUNNER_USER`, `RUNNER_DIR`, `REPO_URL`.

## 3. Turn it on

Jobs only move to the runner when the repository variable `SELF_HOSTED_RUNNER` is `true`:

```sh
gh variable set SELF_HOSTED_RUNNER --body true --repo Unfield/Tern
```

This is also the **off switch**. If the server is down, jobs targeting it would wait in the queue forever; set the variable to `false` and CI falls back to GitHub-hosted runners immediately:

```sh
gh variable set SELF_HOSTED_RUNNER --body false --repo Unfield/Tern
```

Forks never have this variable, so CI on forks always uses GitHub-hosted runners.

## Operations

| Task | How |
|---|---|
| Check status | Repo → Settings → Actions → Runners, or `ssh root@SERVER_IP 'cd /opt/actions-runner && ./svc.sh status'` |
| Logs | `ssh root@SERVER_IP 'journalctl -u "actions.runner.*" -f'` |
| Runner updates | Automatic: the runner updates itself |
| OS updates | Automatic security updates; reboot occasionally (`ssh root@SERVER_IP reboot`) |
| Re-install or move | Run step 2 again; `--replace` keeps the same runner name |
| Free disk space | `ssh root@SERVER_IP 'rm -rf /home/runner/.cache/go-build /opt/actions-runner/_work/_tool/*'` |
| Remove | `ssh root@SERVER_IP 'cd /opt/actions-runner && ./svc.sh stop && ./svc.sh uninstall'`, then delete the runner in repo settings and delete the server |

## Why a long-lived runner?

Each job reuses the same machine, so Go and pnpm caches stay warm and runs are fast. That is only acceptable because the runner never sees untrusted code. If Tern ever needs to run pull requests on self-hosted hardware (for example, SSR benchmarks on stable machines), switch to ephemeral runners: one fresh VM per job, registered with `--ephemeral` and deleted afterwards.
