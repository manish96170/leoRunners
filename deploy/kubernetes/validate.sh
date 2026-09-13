#!/bin/sh
set -eu

base_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
required='kustomization.yaml namespace.yaml serviceaccount.yaml configmap.yaml secret.yaml pvc.yaml deployment.yaml service.yaml README.md'

for file in $required; do
  test -f "$base_dir/$file" || { printf 'missing %s\n' "$file" >&2; exit 1; }
done

grep -q 'runAsNonRoot: true' "$base_dir/deployment.yaml"
grep -q 'runAsUser: 65532' "$base_dir/deployment.yaml"
grep -q 'runAsGroup: 65532' "$base_dir/deployment.yaml"
grep -q 'allowPrivilegeEscalation: false' "$base_dir/deployment.yaml"
grep -q 'readOnlyRootFilesystem: true' "$base_dir/deployment.yaml"
grep -q -- '- ALL' "$base_dir/deployment.yaml"
grep -q 'path: /healthz' "$base_dir/deployment.yaml"
grep -q 'terminationGracePeriodSeconds: 90' "$base_dir/deployment.yaml"
grep -q 'GITHUB_WEBHOOK_SECRET' "$base_dir/deployment.yaml"
grep -q 'secretKeyRef:' "$base_dir/deployment.yaml"
grep -q 'automountServiceAccountToken: false' "$base_dir/serviceaccount.yaml"
grep -q 'replicas: 1' "$base_dir/deployment.yaml"
grep -q 'requests:' "$base_dir/deployment.yaml"
grep -q 'limits:' "$base_dir/deployment.yaml"

if grep -E -R -n -i --include='*.yaml' --include='*.yml' 'gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|encoded_jit_config|jit_config' "$base_dir"; then
  printf 'possible credential or JIT material found in manifests\n' >&2
  exit 1
fi

if command -v kubectl >/dev/null 2>&1; then
  kubectl kustomize "$base_dir" >/dev/null
else
  printf '%s\n' 'kubectl unavailable; structural offline checks passed' >&2
fi

printf '%s\n' 'kubernetes manifest validation passed'
