#!/bin/bash
# First boot copies /etc/skel onto the home PVC. Nothing in the image under
# /home/agent is visible once that volume is mounted.
set -euo pipefail

if [[ ! -f /home/agent/.initialized ]]; then
  # Source path ends in /. so GNU cp copies dotfiles into the mount, not a nested skel/.
  cp -R /etc/skel/. /home/agent/
  mkdir -p /home/agent/work /home/agent/.config
  touch /home/agent/.initialized
fi

# Moshi hooks: when the agent-credentials Secret carries MOSHI_PAIRING_TOKEN,
# pair this agent once (secret saved on the home volume), make sure the
# Claude/Codex/grok hooks are installed, and keep the daemon running.
if [[ -n "${MOSHI_PAIRING_TOKEN:-}" ]] && command -v moshi-hook >/dev/null; then
  state="$HOME/.local/state"
  mkdir -p "$state"
  if [[ ! -f "$HOME/.config/moshi/secrets.json" ]]; then
    moshi-hook pair --token "$MOSHI_PAIRING_TOKEN" --name "${HOSTNAME:-agent}" --store file \
      >>"$state/moshi-hook.log" 2>&1 || echo "moshi-hook pair failed; see $state/moshi-hook.log" >&2
  fi
  moshi-hook install --target claude,codex,grok >>"$state/moshi-hook.log" 2>&1 || true
  (
    unset MOSHI_PAIRING_TOKEN
    export MOSHI_HERDR_PATH=/usr/local/bin/herdr
    while true; do
      moshi-hook serve >>"$state/moshi-hook.log" 2>&1 || true
      sleep 5
    done
  ) &
fi

# podman run <image> <cmd> appends <cmd> to this entrypoint. A Sandbox passes none.
if [[ $# -gt 0 ]]; then
  exec "$@"
fi

exec sleep infinity
