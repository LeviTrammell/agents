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

# podman run <image> <cmd> appends <cmd> to this entrypoint. A Sandbox passes none.
if [[ $# -gt 0 ]]; then
  exec "$@"
fi

exec sleep infinity
