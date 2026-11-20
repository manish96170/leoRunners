# Shared settings and helpers for the local rehearsal. Source it; do not run it.
#
# SAFETY: every AWS call made through this file goes to the local emulator.
# All configured AWS profiles and credentials are hidden, so nothing here can
# reach a real AWS account.

REHEARSAL_DIR=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$REHEARSAL_DIR/../.." && pwd)
REHEARSAL_HOME=${LEO_REHEARSAL_HOME:-$HOME/.leo-runners-rehearsal}
FLOCI_CONTAINER=${LEO_FLOCI_CONTAINER:-leo-floci}
FLOCI_IMAGE=${LEO_FLOCI_IMAGE:-floci/floci:latest}
FLOCI_PORT=${LEO_FLOCI_PORT:-4566}
CONTROLLER_PORT=${LEO_CONTROLLER_PORT:-18090}
WEBHOOK_SECRET=${LEO_WEBHOOK_SECRET:-rehearsal-webhook-secret}
TABLE_NAME=${LEO_TABLE_NAME:-leo-rehearsal-state}
LAUNCH_TEMPLATE_NAME=${LEO_LAUNCH_TEMPLATE_NAME:-leo-rehearsal-runner}
# Floci runs instances as Docker containers. Emulated x86 on an Arm host is many
# times slower, so default to the architecture of the Docker host.
RUNNER_ARCH=${LEO_RUNNER_ARCH:-auto}

umask 077
mkdir -p "$REHEARSAL_HOME"

unset AWS_PROFILE AWS_DEFAULT_PROFILE AWS_SESSION_TOKEN AWS_SECURITY_TOKEN \
  AWS_ROLE_ARN AWS_WEB_IDENTITY_TOKEN_FILE AWS_CONTAINER_CREDENTIALS_FULL_URI \
  AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
export AWS_CONFIG_FILE=/dev/null
export AWS_SHARED_CREDENTIALS_FILE=/dev/null
export AWS_ACCESS_KEY_ID=000000000000
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export AWS_DEFAULT_REGION=us-east-1
export AWS_ENDPOINT_URL="http://127.0.0.1:${FLOCI_PORT}"

log() { printf '%s\n' "rehearsal: $*" >&2; }
die() { log "ERROR: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"; }

resolve_arch() {
  if [ "$RUNNER_ARCH" = auto ]; then
    case "$(docker info --format '{{.Architecture}}' 2>/dev/null)" in
      aarch64 | arm64) RUNNER_ARCH=arm64 ;;
      *) RUNNER_ARCH=x64 ;;
    esac
  fi
  case "$RUNNER_ARCH" in
    arm64) RUNNER_AMI=ami-amazonlinux2023-arm64; RUNNER_INSTANCE_TYPE=${LEO_INSTANCE_TYPE:-t4g.medium}; RUNNER_ARCH_LABEL=arm64 ;;
    x64) RUNNER_AMI=ami-amazonlinux2023; RUNNER_INSTANCE_TYPE=${LEO_INSTANCE_TYPE:-t3.medium}; RUNNER_ARCH_LABEL=x64 ;;
    *) die "LEO_RUNNER_ARCH must be auto, arm64, or x64" ;;
  esac
}

require_floci() {
  need aws; need curl
  curl -fsS -m 3 "$AWS_ENDPOINT_URL/_localstack/health" >/dev/null 2>&1 \
    || curl -fsS -m 3 -o /dev/null "$AWS_ENDPOINT_URL" 2>/dev/null \
    || die "floci is not reachable at $AWS_ENDPOINT_URL; run floci.sh up"
  # Prove the endpoint is the emulator and not a real account.
  account=$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)
  [ "$account" = "000000000000" ] || die "endpoint did not answer as the local emulator (account=$account); refusing to continue"
}

load_state() {
  [ -f "$REHEARSAL_HOME/state.env" ] || die "run setup.sh first"
  # shellcheck disable=SC1091
  . "$REHEARSAL_HOME/state.env"
}
