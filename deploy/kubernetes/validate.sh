#!/bin/sh
set -eu

base_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
required='kustomization.yaml namespace.yaml serviceaccount.yaml configmap.yaml secret.yaml pvc.yaml deployment.yaml service.yaml README.md'
rendered_file=$(mktemp "${TMPDIR:-/tmp}/leo-runners-kustomize.XXXXXX")
cleanup() { rm -f "$rendered_file"; }
trap cleanup EXIT INT TERM HUP

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
grep -q 'cpu:' "$base_dir/deployment.yaml"
grep -q 'memory:' "$base_dir/deployment.yaml"
grep -q 'seccompProfile:' "$base_dir/deployment.yaml"
grep -q 'type: RuntimeDefault' "$base_dir/deployment.yaml"
grep -q 'mountPath: /tmp' "$base_dir/deployment.yaml"
grep -q 'type: ClusterIP' "$base_dir/service.yaml"
grep -q 'targetPort: http' "$base_dir/service.yaml"
grep -q 'digest:' "$base_dir/kustomization.yaml"
if grep -E -n 'newTag: (latest|replace-with-digest|[^[:space:]]+)' "$base_dir/kustomization.yaml"; then
  printf '%s\n' 'immutable image check failed: use an image digest, not a tag' >&2
  exit 1
fi

if grep -E -R -n --include='*.yaml' --include='*.yml' \
    '^kind: (Role|RoleBinding|ClusterRole|ClusterRoleBinding|PodSecurityPolicy)$' "$base_dir"; then
  printf '%s\n' 'RBAC check failed: Kubernetes RBAC resources are not allowed' >&2
  exit 1
fi

if grep -E -R -n -i --include='*.yaml' --include='*.yml' 'gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|BEGIN (RSA|OPENSSH|EC) PRIVATE KEY|encoded_jit_config|jit_config' "$base_dir"; then
  printf 'possible credential or JIT material found in manifests\n' >&2
  exit 1
fi

if command -v kubectl >/dev/null 2>&1; then
  kubectl kustomize "$base_dir" >"$rendered_file"
  rendered_count=$(grep -c '^kind:' "$rendered_file" || true)
  [ "$rendered_count" -ge 8 ] || { printf 'rendered manifest check failed: expected at least 8 resources, got %s\n' "$rendered_count" >&2; exit 1; }
  grep -q '^kind: Namespace$' "$rendered_file"
  grep -q '^kind: ServiceAccount$' "$rendered_file"
  grep -q '^kind: ConfigMap$' "$rendered_file"
  grep -q '^kind: Secret$' "$rendered_file"
  grep -q '^kind: PersistentVolumeClaim$' "$rendered_file"
  grep -q '^kind: Deployment$' "$rendered_file"
  grep -q '^kind: Service$' "$rendered_file"
  grep -q 'image: .*@sha256:' "$rendered_file"
  grep -q 'readOnlyRootFilesystem: true' "$rendered_file"
  grep -q 'runAsNonRoot: true' "$rendered_file"
  if grep -E -q '^kind: (Role|RoleBinding|ClusterRole|ClusterRoleBinding|PodSecurityPolicy)$' "$rendered_file"; then
    printf '%s\n' 'rendered RBAC check failed: forbidden RBAC resource present' >&2
    exit 1
  fi
  printf '%s\n' 'kubectl rendered-manifest checks passed'
else
  printf '%s\n' 'kubectl unavailable; rendered-manifest checks skipped (offline structural checks passed)' >&2
fi

printf '%s\n' 'kubernetes manifest validation passed'
