#!/usr/bin/env bash
# Installs, upgrades, or removes the ForgeDeck Agent on a Docker host (run as root).
#
#   install.sh --url https://forgedeck.example.com                       # downloads the latest release
#   install.sh --url https://forgedeck.example.com --binary ./forgedeck-agent
#   install.sh                                                           # upgrade (already enrolled)
#   install.sh --uninstall [--purge]
#
# Options:
#   --url URL         the Control Plane (needed the first time, to enroll)
#   --binary PATH     install this forgedeck-agent binary instead of downloading one
#   --version TAG     the release to download, such as v0.1.0 (default: the latest)
#   --allow-insecure  accept a non-HTTPS URL (only on a network you trust)
#   --uninstall       stop and remove the service and the binary; keeps the
#                     identity and the user unless --purge is given
#   --purge           with --uninstall, also delete the Agent's identity and user
#   --dry-run         print what would be done and change nothing
#
# The enrollment token (from the dashboard: Servers > Add server) is read from
# FORGEDECK_ENROLLMENT_TOKEN or typed at a hidden prompt. It is never taken as
# an argument, so it does not end up in shell history or process listings.
#
# Without --binary it downloads the binary for this CPU from the project's GitHub
# release and checks it against the release's SHA256SUMS. That protects against a
# corrupted download, not against a compromised release: the checksums come from
# the same place. FORGEDECK_RELEASE_BASE overrides the download address (mirrors).
#
# Running it again is safe: it upgrades the binary and the unit and restarts
# the service, and it does not enroll a second time.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
user=forgedeck-agent
state_dir=/var/lib/forgedeck-agent
binary_dest=/usr/local/bin/forgedeck-agent
unit_dest=/etc/systemd/system/forgedeck-agent.service

release_repo=jakkayy/ForgeDeck
url=""
binary=""
version=""
insecure=""
uninstall=""
purge=""
dry=""

die() { echo "error: $*" >&2; exit 1; }
say() { echo "==> $*"; }
act() {
  if [ -n "$dry" ]; then echo "[dry-run] $*"; else "$@"; fi
}

while [ $# -gt 0 ]; do
  case $1 in
    --url) url=${2:?--url needs a value}; shift 2 ;;
    --binary) binary=${2:?--binary needs a value}; shift 2 ;;
    --version) version=${2:?--version needs a value}; shift 2 ;;
    --allow-insecure) insecure=--allow-insecure; shift ;;
    --uninstall) uninstall=1; shift ;;
    --purge) purge=1; shift ;;
    --dry-run) dry=1; shift ;;
    -h | --help) sed -n '2,32p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
done

if [ -z "$dry" ] && [ "$(id -u)" -ne 0 ]; then
  die "run as root (sudo $0 ...)"
fi
[ "$(uname -s)" = Linux ] || die "the Agent installer supports Linux with systemd"
[ -n "$dry" ] || command -v systemctl >/dev/null || die "systemd is required"

if [ -n "$uninstall" ]; then
  say "stopping and removing the service"
  act systemctl disable --now forgedeck-agent 2>/dev/null || true
  act rm -f "$unit_dest" "$binary_dest"
  act systemctl daemon-reload
  if [ -n "$purge" ]; then
    say "deleting the Agent's identity and user"
    act rm -rf "$state_dir"
    act userdel "$user" 2>/dev/null || true
    echo "Remove this server in the dashboard too: the credential is revoked there."
  else
    echo "Kept $state_dir (the Agent's identity) and the $user user. Add --purge to delete them."
  fi
  exit 0
fi

workdir=""
cleanup() { [ -z "$workdir" ] || rm -rf "$workdir"; }
trap cleanup EXIT

fetch() { # fetch URL DEST
  if command -v curl >/dev/null; then
    curl -fsSL --retry 3 -o "$2" "$1" || die "could not download $1"
  elif command -v wget >/dev/null; then
    wget -q -O "$2" "$1" || die "could not download $1"
  else
    die "curl or wget is needed to download the Agent (or pass --binary PATH)"
  fi
}

unit_src="$here/forgedeck-agent.service"
if [ -z "$binary" ]; then
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no prebuilt Agent for $(uname -m); build one and pass --binary PATH" ;;
  esac
  if [ -n "${FORGEDECK_RELEASE_BASE:-}" ]; then
    base=${FORGEDECK_RELEASE_BASE%/}
  elif [ -n "$version" ]; then
    base="https://github.com/$release_repo/releases/download/$version"
  else
    base="https://github.com/$release_repo/releases/latest/download"
  fi
  name="forgedeck-agent-linux-$arch"
  if [ -n "$dry" ]; then
    echo "[dry-run] download $base/$name and $base/SHA256SUMS, verify, install"
    binary=/dry-run/$name
  else
    command -v sha256sum >/dev/null || die "sha256sum is needed to verify the download"
    workdir=$(mktemp -d)
    say "downloading $name from $base"
    fetch "$base/$name" "$workdir/$name"
    fetch "$base/SHA256SUMS" "$workdir/SHA256SUMS"
    [ -f "$unit_src" ] || { fetch "$base/forgedeck-agent.service" "$workdir/forgedeck-agent.service"; unit_src="$workdir/forgedeck-agent.service"; }
    for f in "$name" forgedeck-agent.service; do
      [ -f "$workdir/$f" ] || continue
      (cd "$workdir" && grep -F " $f" SHA256SUMS | sha256sum -c --quiet -) || die "checksum mismatch for $f: refusing to install it"
    done
    chmod +x "$workdir/$name"
    binary="$workdir/$name"
    say "checksum verified"
  fi
fi
[ -n "$dry" ] || [ -f "$binary" ] || die "no such file: $binary"
if [ -z "$dry" ]; then
  command -v docker >/dev/null || die "Docker is not installed on this host"
  docker compose version >/dev/null 2>&1 || die "the Docker Compose plugin is required (docker compose version failed)"
  getent group docker >/dev/null || die "there is no 'docker' group; is Docker installed from your distribution or docker.com?"
fi

already_enrolled=""
if [ -f "$state_dir/identity.json" ]; then
  already_enrolled=1
elif [ -z "$url" ]; then
  die "this host is not enrolled yet: give --url https://<your ForgeDeck>"
fi

say "creating the $user user"
if ! id -u "$user" >/dev/null 2>&1; then
  act useradd --system --home-dir "$state_dir" --no-create-home --shell /usr/sbin/nologin "$user"
fi
act usermod -aG docker "$user"

say "installing the binary and the service"
act install -m 0755 "$binary" "$binary_dest"
act install -m 0644 "$unit_src" "$unit_dest"
act systemctl daemon-reload
# systemd creates StateDirectory= only at first start; enrolling needs it earlier
act install -d -m 0700 -o "$user" -g "$user" "$state_dir"

if [ -n "$already_enrolled" ]; then
  say "already enrolled: keeping the existing identity"
else
  say "enrolling with $url"
  token="${FORGEDECK_ENROLLMENT_TOKEN:-}"
  if [ -z "$token" ] && [ -z "$dry" ]; then
    [ -t 0 ] || die "set FORGEDECK_ENROLLMENT_TOKEN, or run this from a terminal to be asked for it"
    read -rsp "Enrollment token (from the dashboard, Servers > Add server): " token
    echo
  fi
  # The token goes in on stdin, never in an argument.
  if [ -n "$dry" ]; then
    echo "[dry-run] (token on stdin) runuser -u $user -- $binary_dest enroll --url $url --state-dir $state_dir $insecure"
  else
    printf '%s\n' "$token" | runuser -u "$user" -- "$binary_dest" enroll --url "$url" --state-dir "$state_dir" $insecure
  fi
fi

say "starting the Agent"
act systemctl enable forgedeck-agent
act systemctl restart forgedeck-agent
if [ -z "$dry" ]; then
  sleep 2
  if systemctl is-active --quiet forgedeck-agent; then
    echo "The Agent is running. It appears as online in the dashboard within a minute."
    echo "Logs: journalctl -u forgedeck-agent -f"
  else
    systemctl status forgedeck-agent --no-pager || true
    die "the Agent did not start; see the output above"
  fi
fi
