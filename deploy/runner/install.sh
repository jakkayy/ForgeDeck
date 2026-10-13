#!/usr/bin/env bash
# Installs the ForgeDeck Runner on a dedicated build host (run as root).
#   sudo ./install.sh /path/to/forgedeck-runner
set -euo pipefail

binary="${1:?usage: install.sh /path/to/forgedeck-runner}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

id -u forgedeck-runner >/dev/null 2>&1 || useradd --system --create-home --shell /usr/sbin/nologin forgedeck-runner
install -m 0755 "$binary" /usr/local/bin/forgedeck-runner

install -d -m 0750 -o root -g forgedeck-runner /etc/forgedeck-runner
if [ ! -f /etc/forgedeck-runner/runner.env ]; then
  install -m 0640 -o root -g forgedeck-runner "$here/runner.env.example" /etc/forgedeck-runner/runner.env
  echo "Edit /etc/forgedeck-runner/runner.env: set FORGEDECK_URL and FORGEDECK_RUNNER_CREDENTIAL."
fi
install -m 0644 "$here/forgedeck-runner.service" /etc/systemd/system/forgedeck-runner.service
systemctl daemon-reload

cat <<'MSG'

Installed. Before starting the Runner, give the forgedeck-runner user its own
Docker daemon (recommended; see deploy/runner/README.md):

  sudo apt-get install -y docker-ce-rootless-extras uidmap
  sudo loginctl enable-linger forgedeck-runner
  sudo -u forgedeck-runner -H bash -c 'XDG_RUNTIME_DIR=/run/user/$(id -u) dockerd-rootless-setuptool.sh install'

Then: sudo systemctl enable --now forgedeck-runner
MSG
