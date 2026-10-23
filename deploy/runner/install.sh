#!/usr/bin/env bash
# Installs, upgrades, or removes the ForgeDeck Runner on a dedicated build host (run as root).
#
#   install.sh --url https://forgedeck.example.com                       # downloads the latest release
#   install.sh --url https://forgedeck.example.com --binary ./forgedeck-runner
#   install.sh                                                           # upgrade (already configured)
#   install.sh --uninstall
#
# Options:
#   --url URL         the Control Plane the Runner takes builds from
#   --binary PATH     install this forgedeck-runner binary instead of downloading one
#   --version TAG     the release to download, such as v0.1.0 (default: the latest)
#   --uninstall       stop and remove the service and the binary; keeps /etc/forgedeck-runner
#   --dry-run         print what would be done and change nothing
#
# The Runner's credential (shown once when it is created in the dashboard under
# Runners) is read from FORGEDECK_RUNNER_CREDENTIAL or typed at a hidden prompt, and
# is written to /etc/forgedeck-runner/runner.env, readable only by root and the Runner.
# It is never taken as an argument, so it does not end up in shell history or `ps`.
#
# Without --binary it downloads the binary for this CPU from the project's GitHub
# release and checks it against the release's SHA256SUMS: that catches a corrupted
# download, not a compromised release. FORGEDECK_RELEASE_BASE overrides the address.
#
# Running it again is safe: it upgrades the binary and the unit, keeps runner.env, and restarts.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
user=forgedeck-runner
env_dir=/etc/forgedeck-runner
env_file=$env_dir/runner.env
binary_dest=/usr/local/bin/forgedeck-runner
unit_dest=/etc/systemd/system/forgedeck-runner.service

release_repo=jakkayy/ForgeDeck
url=""
binary=""
version=""
uninstall=""
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
    --uninstall) uninstall=1; shift ;;
    --dry-run) dry=1; shift ;;
    -h | --help) sed -n '2,25p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option $1 (see --help)" ;;
  esac
done

if [ -z "$dry" ] && [ "$(id -u)" -ne 0 ]; then
  die "run as root (sudo $0 ...)"
fi
[ "$(uname -s)" = Linux ] || die "the Runner installer supports Linux with systemd"
[ -n "$dry" ] || command -v systemctl >/dev/null || die "systemd is required"

if [ -n "$uninstall" ]; then
  say "stopping and removing the service"
  act systemctl disable --now forgedeck-runner 2>/dev/null || true
  act rm -f "$unit_dest" "$binary_dest"
  act systemctl daemon-reload
  echo "Kept $env_dir (the credential) and the $user user. Remove the Runner in the dashboard to revoke its credential."
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
    die "curl or wget is needed to download the Runner (or pass --binary PATH)"
  fi
}

unit_src="$here/forgedeck-runner.service"
if [ -z "$binary" ]; then
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "no prebuilt Runner for $(uname -m); build one and pass --binary PATH" ;;
  esac
  if [ -n "${FORGEDECK_RELEASE_BASE:-}" ]; then
    base=${FORGEDECK_RELEASE_BASE%/}
  elif [ -n "$version" ]; then
    base="https://github.com/$release_repo/releases/download/$version"
  else
    base="https://github.com/$release_repo/releases/latest/download"
  fi
  name="forgedeck-runner-linux-$arch"
  if [ -n "$dry" ]; then
    echo "[dry-run] download $base/$name and $base/SHA256SUMS, verify, install"
    binary=/dry-run/$name
  else
    command -v sha256sum >/dev/null || die "sha256sum is needed to verify the download"
    workdir=$(mktemp -d)
    say "downloading $name from $base"
    fetch "$base/$name" "$workdir/$name"
    fetch "$base/SHA256SUMS" "$workdir/SHA256SUMS"
    [ -f "$unit_src" ] || { fetch "$base/forgedeck-runner.service" "$workdir/forgedeck-runner.service"; unit_src="$workdir/forgedeck-runner.service"; }
    for f in "$name" forgedeck-runner.service; do
      [ -f "$workdir/$f" ] || continue
      (cd "$workdir" && grep -F " $f" SHA256SUMS | sha256sum -c --quiet -) || die "checksum mismatch for $f: refusing to install it"
    done
    chmod +x "$workdir/$name"
    binary="$workdir/$name"
    say "checksum verified"
  fi
fi
[ -n "$dry" ] || [ -f "$binary" ] || die "no such file: $binary"

configured=""
[ -f "$env_file" ] && grep -q '^FORGEDECK_RUNNER_CREDENTIAL=fdr_' "$env_file" && configured=1
if [ -z "$configured" ] && [ -z "$url" ]; then
  die "this host is not configured yet: give --url https://<your ForgeDeck>"
fi

say "creating the $user user"
if ! id -u "$user" >/dev/null 2>&1; then
  act useradd --system --create-home --shell /usr/sbin/nologin "$user"
fi

say "installing the binary and the service"
act install -m 0755 "$binary" "$binary_dest"
act install -d -m 0750 -o root -g "$user" "$env_dir"
act install -m 0644 "$unit_src" "$unit_dest"
act systemctl daemon-reload

if [ -n "$configured" ]; then
  say "already configured: keeping $env_file"
else
  credential="${FORGEDECK_RUNNER_CREDENTIAL:-}"
  if [ -z "$credential" ] && [ -z "$dry" ]; then
    [ -t 0 ] || die "set FORGEDECK_RUNNER_CREDENTIAL, or run this from a terminal to be asked for it"
    read -rsp "Runner credential (from the dashboard, Runners > Add Runner): " credential
    echo
  fi
  case $credential in fdr_*) ;; *) [ -n "$dry" ] || die "that does not look like a Runner credential (it starts with fdr_)" ;; esac
  say "writing $env_file"
  if [ -n "$dry" ]; then
    echo "[dry-run] write $env_file (mode 0640, root:$user) with the URL and the credential"
  else
    umask 027
    {
      echo "FORGEDECK_URL=$url"
      echo "FORGEDECK_RUNNER_CREDENTIAL=$credential"
      echo "FORGEDECK_BUILD_TIMEOUT=20m"
      echo "FORGEDECK_TEST_CPUS=2"
      echo "FORGEDECK_TEST_MEMORY=2g"
      echo "FORGEDECK_TEST_PIDS=512"
    } > "$env_file"
    chown root:"$user" "$env_file"
    chmod 0640 "$env_file"
  fi
fi

cat <<'MSG'

The Runner builds untrusted code, so give the forgedeck-runner user its own
Docker daemon before starting it (recommended; see deploy/runner/README.md):

  sudo apt-get install -y docker-ce-rootless-extras uidmap
  sudo loginctl enable-linger forgedeck-runner
  sudo -u forgedeck-runner -H bash -c 'XDG_RUNTIME_DIR=/run/user/$(id -u) dockerd-rootless-setuptool.sh install'

Then: sudo systemctl enable --now forgedeck-runner
MSG
