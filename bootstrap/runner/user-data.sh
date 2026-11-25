#!/bin/bash
# LeoRunners ephemeral runner user-data (generic, no customer values).
#
# The controller replaces the placeholder below with GitHub's one-time
# encoded_jit_config, shell-quoted, and base64-encodes the result for EC2. This
# file must contain the placeholder exactly once.
#
# What it does, in order:
#   1. use a pre-baked runner in RUNNER_HOME if the image has one; otherwise
#      download the pinned runner release and verify its SHA-256 before
#      extracting anything;
#   2. create an unprivileged user (the runner refuses to run as root);
#   3. hand the one-time JIT config to that user through a private file that is
#      deleted as soon as it is read;
#   4. run exactly one job (JIT runners are ephemeral), then power off so the
#      instance does not outlive its job.
#
# Never enable shell tracing in this file: it would print the JIT config into
# the instance logs.
# Production images should pre-bake the runner (ami/) instead of downloading it
# at every launch; this file is the fallback and the local/rehearsal path.
set -euo pipefail
umask 077

JIT_CONFIG_B64={{GITHUB_JIT_CONFIG_B64}}

# Pinned runner release. Update the version and every hash together, using
# bootstrap/runner/pin-latest-runner.sh. GitHub stops accepting registrations
# from runners that are too old, so re-pin regularly.
RUNNER_VERSION="2.338.2"
RUNNER_SHA256_X64="baebf204307bac6193480852732f0aba2ae97b6c5597c244f4d5558390147c40"
RUNNER_SHA256_ARM64="670adb2f268fe21dcbb58b3db0e0aa9d5b3bddfe2c651481336e561ff86de945"

RUNNER_USER="runner"
RUNNER_HOME="/opt/actions-runner"

log() { printf '%s\n' "leo-runner-userdata: $*" >&2; }
die() { log "ERROR: $*"; exit 1; }

[ "$(id -u)" -eq 0 ] || die "must run as root"
[ -n "$JIT_CONFIG_B64" ] || die "no JIT configuration was provided"

case "$(uname -m)" in
  x86_64 | amd64) runner_arch="x64"; runner_sha256="$RUNNER_SHA256_X64" ;;
  aarch64 | arm64) runner_arch="arm64"; runner_sha256="$RUNNER_SHA256_ARM64" ;;
  *) die "unsupported CPU architecture: $(uname -m)" ;;
esac

install_packages() {
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y tar gzip shadow-utils util-linux ca-certificates
    command -v curl >/dev/null 2>&1 || dnf install -y curl-minimal
  elif command -v yum >/dev/null 2>&1; then
    yum install -y tar gzip shadow-utils util-linux ca-certificates curl
  elif command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y --no-install-recommends curl ca-certificates tar gzip passwd util-linux
  else
    die "no supported package manager (dnf, yum, apt-get) to install prerequisites"
  fi
}

missing=0
for tool in curl tar gzip useradd runuser sha256sum; do
  command -v "$tool" >/dev/null 2>&1 || missing=1
done
[ "$missing" -eq 0 ] || install_packages

if [ ! -x "$RUNNER_HOME/run.sh" ]; then
  log "no pre-baked runner found; downloading actions/runner v${RUNNER_VERSION} (${runner_arch})"
  archive="actions-runner-linux-${runner_arch}-${RUNNER_VERSION}.tar.gz"
  work="$(mktemp -d)"
  curl -fsSL --retry 5 --retry-connrefused --retry-delay 2 \
    -o "$work/$archive" \
    "https://github.com/actions/runner/releases/download/v${RUNNER_VERSION}/${archive}"
  # Verify before extracting anything: a mismatch must stop the launch.
  printf '%s  %s\n' "$runner_sha256" "$work/$archive" | sha256sum -c - >/dev/null \
    || die "runner archive checksum mismatch; refusing to run it"
  mkdir -p "$RUNNER_HOME"
  tar -xzf "$work/$archive" -C "$RUNNER_HOME"
  rm -rf "$work"
  # The runner's own dependency installer does not recognise Amazon Linux 2023,
  # so install the .NET prerequisites directly on dnf and yum systems.
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y libicu krb5-libs openssl-libs zlib
  elif command -v yum >/dev/null 2>&1; then
    yum install -y libicu krb5-libs openssl-libs zlib
  else
    "$RUNNER_HOME/bin/installdependencies.sh"
  fi
fi

# Workflow jobs almost always need git (actions/checkout falls back to a slower
# download without it).
if ! command -v git >/dev/null 2>&1; then
  if command -v dnf >/dev/null 2>&1; then dnf install -y git
  elif command -v yum >/dev/null 2>&1; then yum install -y git
  elif command -v apt-get >/dev/null 2>&1; then apt-get install -y --no-install-recommends git
  fi
fi

# Prerequisites most workflows hit on a bare image: unzip (release archives) and
# libatomic, which the Node.js builds that actions/setup-node downloads link
# against on arm64. A missing libatomic shows up as exit code 127 when the job
# tries to run node. Production images should carry both (ami/scripts).
ensure_workload_prerequisites() {
  local pkgs=()
  command -v unzip >/dev/null 2>&1 || pkgs+=(unzip)
  if ! ldconfig -p 2>/dev/null | grep -q 'libatomic\.so\.1'; then
    if command -v apt-get >/dev/null 2>&1; then pkgs+=(libatomic1); else pkgs+=(libatomic); fi
  fi
  [ "${#pkgs[@]}" -eq 0 ] && return 0
  log "installing workload prerequisites: ${pkgs[*]}"
  if command -v dnf >/dev/null 2>&1; then dnf install -y "${pkgs[@]}"
  elif command -v yum >/dev/null 2>&1; then yum install -y "${pkgs[@]}"
  elif command -v apt-get >/dev/null 2>&1; then apt-get install -y --no-install-recommends "${pkgs[@]}"
  fi
}
ensure_workload_prerequisites

id -u "$RUNNER_USER" >/dev/null 2>&1 || useradd --create-home --shell /bin/bash "$RUNNER_USER"
# An image that carries Docker (the front-end profile) needs the runner user in
# its group to use it. Membership is root-equivalent on the host, so it is granted
# only when the image already has Docker; the instance is single-use, private and
# holds no credentials beyond the one-time JIT configuration.
if getent group docker >/dev/null 2>&1; then
  usermod -aG docker "$RUNNER_USER"
fi
chown -R "$RUNNER_USER" "$RUNNER_HOME"

jit_dir="$(mktemp -d)"
chown "$RUNNER_USER" "$jit_dir"
chmod 700 "$jit_dir"
jit_file="$jit_dir/jit"
printf '%s' "$JIT_CONFIG_B64" > "$jit_file"
chown "$RUNNER_USER" "$jit_file"
chmod 600 "$jit_file"
unset JIT_CONFIG_B64

cleanup() {
  rm -rf "$jit_dir"
}
trap cleanup EXIT

log "starting the runner for one job"
status=0
# The runner user reads the config once and deletes it before the runner starts.
# The value is still visible in the runner's argument list while it registers;
# the instance is disposable and the value is single use.
runuser -u "$RUNNER_USER" -- bash -c '
  set -euo pipefail
  cd "$1"
  jit="$(cat "$2")"
  rm -f "$2"
  exec ./run.sh --jitconfig "$jit"
' _ "$RUNNER_HOME" "$jit_file" || status=$?

log "runner exited with status ${status}"
cleanup
# Instances are launched with shutdown behaviour "terminate", so this ends the
# instance. Containers and other environments may not have it; the controller
# also terminates the instance when GitHub reports the job completed.
shutdown -h now 2>/dev/null || poweroff 2>/dev/null || true
exit "$status"
