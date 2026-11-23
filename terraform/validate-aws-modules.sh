#!/usr/bin/env bash
# Offline checks for the generic AWS deployment modules and the example
# composition. Makes no AWS calls and needs no credentials: tests use a mocked
# AWS provider. `terraform init` downloads the provider plugin from the public
# registry; pass --skip-init to run only the static checks.
#
# The modules declare AWS provider ">= 5.0, < 7.0". Pass
# --provider-version <x.y.z> to run every check against exactly that provider
# release (in a temporary copy), so both ends of the range can be verified:
#   terraform/validate-aws-modules.sh --provider-version 5.100.0
#   terraform/validate-aws-modules.sh --provider-version 6.73.0
set -euo pipefail

root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
skip_init=false
provider_version=
while [ $# -gt 0 ]; do
  case "$1" in
    --skip-init) skip_init=true ;;
    --provider-version)
      [ $# -ge 2 ] || { echo '--provider-version needs a value' >&2; exit 2; }
      provider_version=$2; shift ;;
    *) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

if [ -n "$provider_version" ]; then
  [[ "$provider_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'provider version must look like 5.100.0' >&2; exit 2; }
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  # Keep the repository layout: the runner layer reads the bundled bootstrap
  # script from outside terraform/, as it does when consumed from a Git source.
  mkdir -p "$tmp/terraform" "$tmp/bootstrap/runner"
  tar -C "$root" --exclude=.terraform -cf - . | tar -C "$tmp/terraform" -xf -
  cp "$root/../bootstrap/runner/user-data.sh" "$tmp/bootstrap/runner/user-data.sh"
  find "$tmp/terraform" -name '*.tf' -exec sed -i.bak "s/version *= *\">= 5.0, < 7.0\"/version = \"= $provider_version\"/" {} +
  find "$tmp/terraform" -name '*.bak' -delete
  find "$tmp/terraform" -name .terraform.lock.hcl -delete
  printf 'running against AWS provider %s\n' "$provider_version"
  args=()
  [ "$skip_init" = true ] && args+=(--skip-init)
  "$tmp/terraform/validate-aws-modules.sh" ${args[@]+"${args[@]}"}
  exit $?
fi

modules=(aws-ecs-controller aws-shared-gateway aws-ephemeral-runner aws-runner-layer)
dirs=()
for m in "${modules[@]}"; do dirs+=("$root/modules/$m"); done
dirs+=("$root/examples/aws-customer" "$root/environments/aws-foundation")

required=(main.tf variables.tf outputs.tf versions.tf README.md variables.example.tfvars)
for m in "${modules[@]}"; do
  for f in "${required[@]}"; do
    test -f "$root/modules/$m/$f" || { printf 'missing required file: modules/%s/%s\n' "$m" "$f" >&2; exit 2; }
  done
  ls "$root/modules/$m"/tests/*.tftest.hcl >/dev/null 2>&1 || { printf 'missing tests: modules/%s/tests\n' "$m" >&2; exit 2; }
done

scan_paths=("$root/modules/aws-ecs-controller" "$root/modules/aws-shared-gateway" "$root/modules/aws-ephemeral-runner" "$root/modules/aws-runner-layer" "$root/examples" "$root/environments/aws-foundation")

if grep -rEn --exclude-dir=.terraform '(AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|gh[pousr]_[A-Za-z0-9_]{20,})' "${scan_paths[@]}"; then
  echo 'credential-like material found' >&2
  exit 2
fi

# Only the documentation account IDs below may appear in these generic modules.
if grep -rEno --exclude-dir=.terraform --exclude='.terraform.lock.hcl' '[^0-9][0-9]{12}[^0-9]' "${scan_paths[@]}" | grep -vE '111122223333|444455556666|000000000000'; then
  echo 'non-placeholder 12-digit account ID found in generic modules' >&2
  exit 2
fi

if grep -rEn --include='*.tf' '"(Action|Resource)"[[:space:]]*:[[:space:]]*"\*"' "${scan_paths[@]}"; then
  echo 'hand-written wildcard IAM statement found' >&2
  exit 2
fi

if grep -rEn --include='*.tf' 'default[[:space:]]*=[[:space:]]*"(sg|vpc|subnet|ami)-' "${scan_paths[@]}"; then
  echo 'hardcoded network or image default found' >&2
  exit 2
fi

command -v terraform >/dev/null 2>&1 || { echo 'terraform not installed; static checks passed, skipped terraform checks' >&2; exit 0; }

terraform fmt -check -recursive "$root/modules/aws-ecs-controller" "$root/modules/aws-shared-gateway" "$root/modules/aws-ephemeral-runner" "$root/modules/aws-runner-layer" "$root/examples" "$root/environments/aws-foundation"

if [[ "$skip_init" == "true" ]]; then
  echo 'static checks passed (init, validate, and tests skipped)'
  exit 0
fi

for d in "${dirs[@]}"; do
  printf '== %s\n' "${d#"$root"/}"
  (cd "$d" && terraform init -backend=false -input=false >/dev/null && terraform validate && terraform test)
done

# Modules that read the provider's region attribute, which differs between
# provider major versions. Validation alone is enough to catch a regression.
for m in runner-network observability; do
  printf '== modules/%s\n' "$m"
  (cd "$root/modules/$m" && terraform init -backend=false -input=false >/dev/null && terraform validate)
done

echo 'aws module validation passed'
