# agents

Self-hosted coding agents on Kubernetes. Each agent is a
[kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox)
`Sandbox` named `agent-<name>`: one pod plus a persistent home volume, with
pause and resume. You reach agents over SSH through a small
[Wish](https://github.com/charmbracelet/wish) app that drops you into the
agent's [herdr](https://herdr.dev) session.

| Path | What |
|---|---|
| `images/agent-coding/` | The image every agent runs: Fedora with zsh/oh-my-zsh, zoxide, zellij, herdr, neovim, uv, Go, Rust, Node, `gh`, `task`, Claude Code, Codex, grok, and the ACP adapters. |
| `gateway-app/` | `agent-ssh`, the SSH front door (Go, Wish, Bubble Tea, client-go). |
| `bin/agent` | The same operations as a bash CLI, for a machine with a kubeconfig. |

The cluster manifests (controller install, namespace, quota, gateway
Deployment) are not in this repo.

## SSH usage

```bash
ssh levi@agents                          # menu: attach, new, stop, resume, delete
ssh smoke@agents                         # straight into agent "smoke" (resumes it if paused)
ssh new@agents                           # create an agent (name, optional repo), then attach
ssh -N -L 3000:localhost:3000 smoke@agents   # reach a dev server inside "smoke"
```

- Port forwarding goes through the Kubernetes port-forward API, so servers
  bound to `127.0.0.1` inside the agent are reachable. Only loopback
  destinations are allowed, and only on direct agent logins.
- In Moshi or any phone SSH client, save one host per agent (`<name>@agents`)
  for one-tap entry.
- Authentication is public-key only, checked against a mounted
  `authorized_keys`.

## Images

GitHub Actions builds on a native amd64 runner and pushes to
`ghcr.io/levitrammell/agent-coding` and `ghcr.io/levitrammell/agent-ssh`.
Tags: `latest` from `main`, `sha-<short>`, and `YYYYMMDD` on the weekly run.
Never put secrets in an image. Harness credentials live in a Kubernetes
Secret or on each agent's home volume.

## Development

```bash
cd gateway-app
go test ./...        # includes a parity test against bin/agent's manifest
go run . -h
```
