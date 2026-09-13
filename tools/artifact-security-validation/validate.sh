#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
fixture_root=${SECURITY_ARTIFACT_FIXTURES:-$repo_root/validation/security-artifacts}
dockerfile="$fixture_root/safe/Dockerfile"
manifest_dir="$fixture_root/safe"
metadata="$fixture_root/safe/image-metadata.v1.json"
report=
image=
render_dir=

usage() {
  cat <<'USAGE'
Usage: validate.sh [options]
  --dockerfile PATH       Dockerfile to scan
  --manifest-dir PATH     directory containing YAML manifests
  --metadata PATH          built-image metadata JSON contract
  --image NAME             inspect an existing local image when Docker exists
  --render-kustomize PATH  render a Kustomize directory when kubectl exists
  --report PATH            atomically write a redacted JSON report
  --help                   show this help

Offline by default. Docker and kubectl are optional read-only enrichments.
Exit status: 0 for PASS or WARN, 1 for FAIL, 2 for invalid input.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dockerfile) [ "$#" -ge 2 ] || { echo '--dockerfile requires PATH' >&2; exit 2; }; dockerfile=$2; shift ;;
    --manifest-dir) [ "$#" -ge 2 ] || { echo '--manifest-dir requires PATH' >&2; exit 2; }; manifest_dir=$2; shift ;;
    --metadata) [ "$#" -ge 2 ] || { echo '--metadata requires PATH' >&2; exit 2; }; metadata=$2; shift ;;
    --image) [ "$#" -ge 2 ] || { echo '--image requires NAME' >&2; exit 2; }; image=$2; shift ;;
    --render-kustomize) [ "$#" -ge 2 ] || { echo '--render-kustomize requires PATH' >&2; exit 2; }; render_dir=$2; shift ;;
    --report) [ "$#" -ge 2 ] || { echo '--report requires PATH' >&2; exit 2; }; report=$2; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

[ -f "$dockerfile" ] || { echo "Dockerfile not found: $dockerfile" >&2; exit 2; }
[ -d "$manifest_dir" ] || { echo "manifest directory not found: $manifest_dir" >&2; exit 2; }
[ -f "$metadata" ] || { echo "metadata file not found: $metadata" >&2; exit 2; }

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/leo-artifact-security.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT INT TERM HUP
warnings_file="$tmp_dir/tool-warnings"
rendered="$tmp_dir/rendered.yaml"
: > "$warnings_file"

if [ -n "$image" ]; then
  if command -v docker >/dev/null 2>&1; then
    docker image inspect --format '{{json .}}' "$image" > "$tmp_dir/image.json" 2>/dev/null ||
      echo 'WARN docker.inspect_unavailable: requested image metadata could not be read' >> "$warnings_file"
  else
    echo 'WARN docker.unavailable: Docker CLI is not installed; image inspection skipped' >> "$warnings_file"
  fi
fi
if [ -n "$render_dir" ]; then
  if command -v kubectl >/dev/null 2>&1; then
    kubectl kustomize "$render_dir" > "$rendered" 2>/dev/null || {
      echo 'WARN kubectl.render_unavailable: Kustomize rendering failed; static manifests retained' >> "$warnings_file"
      : > "$rendered"
    }
  else
    echo 'WARN kubectl.unavailable: kubectl is not installed; rendering skipped' >> "$warnings_file"
  fi
fi

ruby -ryaml -rjson - "$dockerfile" "$manifest_dir" "$metadata" "$warnings_file" "$rendered" "$report" "$tmp_dir/image.json" <<'RUBY'
dockerfile, manifest_dir, metadata_path, warnings_path, rendered_path, report_path, docker_image_path = ARGV
results = []
add = lambda { |level, code, message| results << {"level" => level, "code" => code, "message" => message} }
abort "artifact security validation input error: invalid metadata JSON" unless (metadata = (JSON.parse(File.read(metadata_path)) rescue nil)).is_a?(Hash)

add.call("FAIL", "metadata.identity", "built-image metadata identity is invalid") unless metadata["apiVersion"] == "security.leorunners.io/v1" && metadata["kind"] == "BuiltImageMetadata"
spec = metadata.fetch("spec", {})
add.call("FAIL", "metadata.digest", "built image digest is not pinned") unless spec["digest"].to_s.match?(/\Asha256:[0-9a-f]{64}\z/)
add.call("FAIL", "metadata.architecture", "built image architecture is not linux/amd64") unless spec["os"] == "linux" && spec["architecture"] == "amd64"
add.call("FAIL", "metadata.non_root", "built image does not declare a non-root user") unless spec["user"].to_s.match?(/\A(?:[1-9][0-9]*|[a-z][a-z0-9._-]*)\z/i)
add.call("FAIL", "metadata.read_only", "built image is not marked read-only compatible") unless spec["readOnlyRootFilesystem"] == true
add.call("FAIL", "metadata.secrets", "built image metadata indicates embedded secrets") unless spec["secretsBakedIn"] == false
add.call("FAIL", "metadata.ai_enabled", "AI is enabled in the built-image defaults") unless spec.dig("ai", "enabled") == false
add.call("WARN", "metadata.provenance", "built-image provenance is incomplete") unless metadata.dig("provenance", "builder") == "pinned-build"

if File.file?(docker_image_path) && File.size?(docker_image_path)
  image = JSON.parse(File.read(docker_image_path)) rescue {}
  image_user = image.dig("Config", "User").to_s
  add.call("FAIL", "image.user", "inspected image declares root or no runtime user") if image_user.empty? || image_user.match?(/\A(?:0|root)(?::0)?\z/i)
  add.call("FAIL", "image.platform", "inspected image is not linux/amd64") unless image["Os"] == "linux" && image["Architecture"] == "amd64"
  add.call("FAIL", "image.digest", "inspected image has no immutable repository digest") unless Array(image["RepoDigests"]).any? { |digest| digest.match?(/@sha256:[0-9a-f]{64}\z/i) }
  Array(image.dig("Config", "Env")).each do |entry|
    add.call("FAIL", "image.secret_literal", "inspected image contains a secret-like environment value") if entry.match?(/(?:AWS_SECRET|AWS_ACCESS_KEY|API_KEY|TOKEN|PASSWORD|PRIVATE_KEY)=.+/i)
    add.call("FAIL", "image.ai_enabled", "inspected image enables AI by default") if entry.match?(/\AAI_ENABLED=(?:true|1|yes)\z/i)
  end
end

docker = File.read(dockerfile)
froms = docker.lines.select { |line| line =~ /^\s*FROM\s+/i }
add.call("FAIL", "docker.no_from", "Dockerfile has no base image") if froms.empty?
froms.each { |line| add.call("FAIL", "docker.digest", "Dockerfile base image is not digest pinned") unless line.match?(/@sha256:[0-9a-f]{64}/i) }
last_user = docker.lines.select { |line| line =~ /^\s*USER\s+/i }.last.to_s.split[1]
add.call("FAIL", "docker.root_user", "Dockerfile final user is root or missing") unless last_user && last_user !~ /\A(?:0|root)\z/i
add.call("FAIL", "docker.secret_literal", "Dockerfile contains a secret-like literal or secret build input") if docker.match?(/(?:AWS_SECRET|AWS_ACCESS_KEY|API_KEY|TOKEN|PASSWORD|PRIVATE_KEY)\s*=/i)
add.call("FAIL", "docker.secret_copy", "Dockerfile copies a credential or environment file") if docker.match?(/(?:COPY|ADD)\s+[^\n]*(?:\.env|credentials|id_rsa|\.pem|\.key)/i)
if docker.match?(/^\s*ENV\s+AI_ENABLED\s*=\s*(?:true|1|yes)\b/i)
  add.call("FAIL", "docker.ai_enabled", "Dockerfile enables AI by default")
elsif !docker.match?(/^\s*ENV\s+AI_ENABLED\s*=\s*(?:false|0|no)\b/i)
  add.call("WARN", "docker.ai_unspecified", "Dockerfile does not explicitly disable AI by default")
end

yaml_files = Dir[File.join(manifest_dir, "**", "*.{yaml,yml}")]
yaml_files += [rendered_path] if File.file?(rendered_path) && File.size?(rendered_path)
documents = []
yaml_files.uniq.each do |path|
  begin
    File.read(path).split(/^---\s*$\n?/).each do |document|
      next if document.strip.empty?
      value = YAML.safe_load(document, permitted_classes: [], aliases: false)
      documents << value unless value.nil?
    end
  rescue StandardError
    add.call("FAIL", "kubernetes.parse", "Kubernetes manifest could not be parsed")
  end
end
add.call("WARN", "kubernetes.empty", "no Kubernetes manifests were supplied") if documents.empty?

walk = lambda do |value, &block|
  block.call(value)
  value.each_value { |child| walk.call(child, &block) } if value.is_a?(Hash)
  value.each { |child| walk.call(child, &block) } if value.is_a?(Array)
end
container_count = 0
documents.each do |doc|
  next unless doc.is_a?(Hash)
  kind = doc["kind"].to_s
  non_root_declared = false
  document_container_count = 0
  if kind == "Secret"
    %w[data stringData].each do |field|
      values = doc[field]
      add.call("FAIL", "kubernetes.secret_exposure", "Kubernetes Secret contains non-placeholder material") if values.is_a?(Hash) && values.any? { |_key, value| !value.to_s.empty? && value.to_s !~ /\A(?:REPLACE|CHANGE_ME|TODO|example|placeholder)/i }
    end
  end
  if %w[Role ClusterRole].include?(kind)
    Array(doc["rules"]).each do |rule|
      add.call("FAIL", "kubernetes.rbac_wildcard", "RBAC rule grants wildcard access") if rule.is_a?(Hash) && (Array(rule["verbs"]).include?("*") || Array(rule["resources"]).include?("*") || Array(rule["apiGroups"]).include?("*"))
    end
  end
  add.call("FAIL", "kubernetes.rbac_admin", "ClusterRoleBinding grants cluster-admin") if kind == "ClusterRoleBinding" && doc.dig("roleRef", "name") == "cluster-admin"
  walk.call(doc) do |node|
    next unless node.is_a?(Hash)
    non_root_declared = true if node["runAsNonRoot"] == true
    add.call("FAIL", "kubernetes.host_access", "Kubernetes manifest requests host or privileged access") if %w[hostNetwork hostPID hostIPC privileged].any? { |key| node[key] == true }
    add.call("FAIL", "kubernetes.host_access", "Kubernetes manifest requests a host port") if node["hostPort"].to_i > 0
    add.call("FAIL", "kubernetes.host_path", "Kubernetes manifest mounts a hostPath") if node.key?("hostPath")
    add.call("FAIL", "kubernetes.service_account_token", "Kubernetes service-account token automount is enabled") if node["automountServiceAccountToken"] == true
    if node.key?("image")
      container_count += 1
      document_container_count += 1
      add.call("FAIL", "kubernetes.image_digest", "Kubernetes container image is not digest pinned") unless node["image"].to_s.match?(/@sha256:[0-9a-f]{64}\z/i)
      security_context = node["securityContext"].is_a?(Hash) ? node["securityContext"] : {}
      add.call("FAIL", "kubernetes.writable_root", "Kubernetes container lacks a read-only root filesystem") unless security_context["readOnlyRootFilesystem"] == true
      add.call("FAIL", "kubernetes.privilege_escalation", "Kubernetes container lacks privilege-escalation denial") unless security_context["allowPrivilegeEscalation"] == false
    end
    add.call("FAIL", "kubernetes.writable_root", "Kubernetes container root filesystem is writable") if node["readOnlyRootFilesystem"] == false
    add.call("FAIL", "kubernetes.privilege_escalation", "Kubernetes privilege escalation is allowed") if node["allowPrivilegeEscalation"] != nil && node["allowPrivilegeEscalation"] != false
    add.call("FAIL", "kubernetes.root_user", "Kubernetes workload can run as root") if node["runAsNonRoot"] == false || (node.key?("runAsUser") && node["runAsUser"].to_i == 0)
    add.call("FAIL", "kubernetes.ai_secret", "Kubernetes manifest contains an AI credential value") if node["name"].to_s.match?(/AI_API_KEY|OPENAI_API_KEY|ANTHROPIC_API_KEY/i) && !node["value"].to_s.empty?
    add.call("FAIL", "kubernetes.ai_enabled", "Kubernetes manifest enables AI by default") if node["name"].to_s == "AI_ENABLED" && node["value"].to_s.match?(/\A(?:true|1|yes)\z/i)
  end
  add.call("FAIL", "kubernetes.non_root_missing", "Kubernetes workload lacks an explicit non-root declaration") if document_container_count > 0 && !non_root_declared
end
add.call("FAIL", "kubernetes.no_containers", "Kubernetes manifests contain no containers") if container_count == 0
File.foreach(warnings_path) { |line| level, code, message = line.strip.split(/\s+/, 3); add.call(level, code, message) unless line.strip.empty? }

status = results.any? { |entry| entry["level"] == "FAIL" } ? "FAIL" : (results.any? { |entry| entry["level"] == "WARN" } ? "WARN" : "PASS")
object = {"apiVersion" => "security.leorunners.io/v1", "kind" => "ArtifactSecurityReport", "status" => status, "checks" => results}
unless report_path.to_s.empty?
  File.umask(0o077)
  temp = "#{report_path}.tmp.#{$$}"
  File.write(temp, JSON.pretty_generate(object) + "\n", mode: "w", perm: 0o600)
  File.rename(temp, report_path)
end
results.each { |entry| puts "#{entry['level']} #{entry['code']}: #{entry['message']}" }
puts "#{status} artifact security validation"
exit(status == "FAIL" ? 1 : 0)
RUBY
