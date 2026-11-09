#!/usr/bin/env bash
#
# Bootstraps a GitHub Actions self-hosted runner for Unfield/Tern on a fresh
# Ubuntu 24.04 server (built for Hetzner Cloud, works on any Ubuntu host).
#
# Run as root, with a registration token in RUNNER_TOKEN. See README.md:
#
#   ssh root@SERVER "RUNNER_TOKEN=$(gh api -X POST repos/Unfield/Tern/actions/runners/registration-token --jq .token) bash -s" < setup.sh
#
# The script is safe to re-run: it upgrades packages, keeps the hardening in
# place and re-registers the runner under the same name.

set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/Unfield/Tern}"
RUNNER_NAME="${RUNNER_NAME:-hetzner-$(hostname -s)}"
RUNNER_LABELS="${RUNNER_LABELS:-hetzner}"
RUNNER_USER="${RUNNER_USER:-runner}"
RUNNER_DIR="${RUNNER_DIR:-/opt/actions-runner}"

log() { printf '\n==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

[[ ${EUID} -eq 0 ]] || die "run this script as root"
[[ -n "${RUNNER_TOKEN:-}" ]] || die "RUNNER_TOKEN is not set (see README.md)"
# shellcheck source=/dev/null
[[ -r /etc/os-release ]] && . /etc/os-release
[[ "${ID:-}" == "ubuntu" ]] || die "this script supports Ubuntu only (found: ${ID:-unknown})"

case "$(uname -m)" in
  x86_64)  ARCH=x64 ;;
  aarch64) ARCH=arm64 ;;
  *)       die "unsupported architecture: $(uname -m)" ;;
esac

log "Installing system packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get upgrade -yq
apt-get install -yq --no-install-recommends \
  build-essential ca-certificates curl fail2ban git jq tar ufw unattended-upgrades unzip

log "Enabling automatic security updates"
cat > /etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF

log "Hardening SSH (keys only)"
cat > /etc/ssh/sshd_config.d/99-hardening.conf <<'EOF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin prohibit-password
EOF
# Ubuntu 24.04 starts sshd via socket activation, so /run/sshd may not exist yet.
mkdir -p /run/sshd
sshd -t
systemctl try-reload-or-restart ssh

log "Configuring firewall (inbound SSH only; the runner only needs outbound HTTPS)"
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw --force enable

log "Creating unprivileged user '${RUNNER_USER}'"
if ! id "${RUNNER_USER}" &>/dev/null; then
  useradd --create-home --shell /bin/bash "${RUNNER_USER}"
fi

log "Downloading the latest GitHub Actions runner (${ARCH})"
release_json="$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest)"
version="$(jq -r '.tag_name' <<<"${release_json}")"
version="${version#v}"
expected_sha="$(jq -r '.body' <<<"${release_json}" \
  | sed -n "s/.*<!-- BEGIN SHA linux-${ARCH} -->\([0-9a-f]\{64\}\)<!-- END SHA linux-${ARCH} -->.*/\1/p")"
[[ -n "${expected_sha}" ]] || die "could not find the published checksum for runner ${version}"

tarball="$(mktemp)"
trap 'rm -f "${tarball}"' EXIT
curl -fsSL -o "${tarball}" \
  "https://github.com/actions/runner/releases/download/v${version}/actions-runner-linux-${ARCH}-${version}.tar.gz"
echo "${expected_sha}  ${tarball}" | sha256sum --check --status || die "checksum mismatch for runner ${version}"

if [[ -x "${RUNNER_DIR}/svc.sh" ]]; then
  log "Removing the previous runner service"
  (cd "${RUNNER_DIR}" && ./svc.sh stop || true; ./svc.sh uninstall || true)
fi

mkdir -p "${RUNNER_DIR}"
tar -xzf "${tarball}" -C "${RUNNER_DIR}"
chown -R "${RUNNER_USER}:" "${RUNNER_DIR}"
"${RUNNER_DIR}/bin/installdependencies.sh"

log "Registering runner '${RUNNER_NAME}' with labels '${RUNNER_LABELS}'"
sudo -u "${RUNNER_USER}" "${RUNNER_DIR}/config.sh" \
  --unattended \
  --replace \
  --url "${REPO_URL}" \
  --token "${RUNNER_TOKEN}" \
  --name "${RUNNER_NAME}" \
  --labels "${RUNNER_LABELS}" \
  --work _work

log "Installing and starting the runner service"
cd "${RUNNER_DIR}"
./svc.sh install "${RUNNER_USER}"
./svc.sh start

log "Done. Runner ${version} '${RUNNER_NAME}' is running."
echo "Check it at ${REPO_URL}/settings/actions/runners"
