#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

sh -n "$script_dir/validate.sh"
sh -n "$script_dir/test.sh"
for file in kustomization.yaml namespace.yaml serviceaccount.yaml configmap.yaml secret.yaml pvc.yaml deployment.yaml service.yaml; do
    test -f "$script_dir/$file"
done
grep -F 'digest: sha256:' "$script_dir/kustomization.yaml" >/dev/null
grep -F 'runAsNonRoot: true' "$script_dir/deployment.yaml" >/dev/null
grep -F 'readOnlyRootFilesystem: true' "$script_dir/deployment.yaml" >/dev/null
grep -F 'type: RuntimeDefault' "$script_dir/deployment.yaml" >/dev/null
grep -F 'targetPort: http' "$script_dir/service.yaml" >/dev/null
if grep -E -R --include='*.yaml' --include='*.yml' \
    '^kind: (Role|RoleBinding|ClusterRole|ClusterRoleBinding|PodSecurityPolicy)$' "$script_dir"; then
    printf '%s\n' 'forbidden RBAC resource found' >&2
    exit 1
fi

"$script_dir/validate.sh"
printf '%s\n' 'Kubernetes manifest tests passed'
