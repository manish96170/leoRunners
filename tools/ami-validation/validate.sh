#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)
contract=${AMI_CONTRACT_FILE:-$repo_root/ami/image-contract.v1.json}
manifest=${AMI_MANIFEST_FILE:-$repo_root/ami/manifest.example.v1.json}
confirm=
var_file=
real_packer=0
build=0

usage() {
  cat <<'USAGE'
Usage: validate.sh [--contract PATH] [--manifest PATH]
       validate.sh --real-packer --confirm I_UNDERSTAND_PACKER_VALIDATION --var-file PATH
       validate.sh --real-packer --build --confirm I_UNDERSTAND_PACKER_BUILD --var-file PATH

Default mode is offline and never invokes Packer or cloud APIs.
--real-packer requires the exact confirmation token and a variables file.
--build additionally runs `packer build` and is intentionally separate.
USAGE
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --contract) [ "$#" -ge 2 ] || { echo '--contract requires PATH' >&2; exit 2; }; contract=$2; shift ;;
    --manifest) [ "$#" -ge 2 ] || { echo '--manifest requires PATH' >&2; exit 2; }; manifest=$2; shift ;;
    --real-packer) real_packer=1 ;;
    --build) build=1 ;;
    --confirm) [ "$#" -ge 2 ] || { echo '--confirm requires TOKEN' >&2; exit 2; }; confirm=$2; shift ;;
    --var-file) [ "$#" -ge 2 ] || { echo '--var-file requires PATH' >&2; exit 2; }; var_file=$2; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$build" -eq 1 ] && [ "$real_packer" -eq 0 ]; then
  echo '--build requires --real-packer' >&2
  exit 2
fi
if [ "$real_packer" -eq 1 ]; then
  [ -n "$var_file" ] && [ -f "$var_file" ] || { echo '--real-packer requires an existing --var-file' >&2; exit 2; }
  [ "$build" -eq 1 ] && expected=I_UNDERSTAND_PACKER_BUILD || expected=I_UNDERSTAND_PACKER_VALIDATION
  [ "$confirm" = "$expected" ] || { echo "exact confirmation token required: $expected" >&2; exit 2; }
fi

for path in "$contract" "$manifest" "$repo_root/ami/runner.pkr.hcl" "$repo_root/ami/profiles/base-linux-x64.yaml"; do
  [ -f "$path" ] || { echo "file not found: $path" >&2; exit 2; }
done
command -v ruby >/dev/null 2>&1 || { echo 'ruby is required for offline AMI validation' >&2; exit 2; }

ruby -ryaml -rjson - "$contract" "$manifest" "$repo_root/ami/runner.pkr.hcl" "$repo_root/ami/profiles/base-linux-x64.yaml" <<'RUBY'
contract_path, manifest_path, hcl_path, profile_path = ARGV
contract = JSON.parse(File.read(contract_path))
manifest = JSON.parse(File.read(manifest_path))
profile = YAML.safe_load(File.read(profile_path), permitted_classes: [], aliases: false)
hcl = File.read(hcl_path)
fail_contract = ->(message) { abort "AMI validation failed: #{message}" }

fail_contract.call("contract apiVersion/kind") unless contract["apiVersion"] == "ami.leorunners.io/v1" && contract["kind"] == "RunnerImageContract"
fail_contract.call("manifest apiVersion/kind") unless manifest["apiVersion"] == "ami.leorunners.io/v1" && manifest["kind"] == "RunnerImageManifest"
spec = contract.fetch("spec")
source = spec.fetch("source")
runtime = spec.fetch("runtime")
artifact = spec.fetch("artifact")
rollback = spec.fetch("rollback")
metadata = contract.fetch("metadata")
version = metadata.fetch("version")
fail_contract.call("contract version") unless version.match?(/\A\d+\.\d+\.\d+\z/)
fail_contract.call("source must be explicitly pinned") unless source["resolution"] == "explicit-pinned-input" && source["ami_id"].match?(/\Aami-[0-9a-f]{8,}\z/)
fail_contract.call("architecture must be x86_64") unless source["architecture"] == "x86_64" && profile.dig("spec", "architecture") == "x86_64"
fail_contract.call("non-root runtime contract") unless runtime["user"] == "runner" && runtime["allow_root_jobs"] == false && profile.dig("spec", "security", "user") == "runner" && profile.dig("spec", "security", "allowRootJobs") == false
fail_contract.call("secret-free image contract") unless runtime["credentials_baked_in"] == false && runtime["jit_config_baked_in"] == false && profile.dig("spec", "security", "credentialsBakedIn") == false && profile.dig("spec", "security", "jitConfigBakedIn") == false
fail_contract.call("IMDSv2 and encrypted root are required") unless runtime["imds_v2_required"] == true && runtime["encrypted_root_volume"] == true && hcl.include?(%q[http_tokens                 = "required"]) && hcl.include?(%q[imds_support = "v2.0"]) && hcl.include?("encrypted             = true")
fail_contract.call("artifact must be immutable and content addressed") unless artifact["immutable"] == true && artifact["digest_source"] == "packer-manifest-and-ami-snapshot" && artifact["digest"].match?(/\Asha256:[0-9a-f]{64}\z/)
fail_contract.call("artifact version mismatch") unless artifact["version"] == version && manifest.dig("metadata", "version") == version && manifest.dig("artifact", "profile_version") == version
fail_contract.call("manifest source or architecture mismatch") unless manifest.dig("artifact", "source_ami") == source["ami_id"] && manifest.dig("artifact", "architecture") == source["architecture"] && manifest.dig("artifact", "digest") == artifact["digest"]
fail_contract.call("manifest provenance") unless manifest.dig("provenance", "builder") == "packer" && manifest.dig("provenance", "package_inventory") == "rpm-query" && manifest.dig("provenance", "source_resolution") == "explicit-pinned-input"
fail_contract.call("rollback metadata") unless rollback["strategy"] == "previous-approved-image" && rollback["retention_count"].is_a?(Integer) && rollback["retention_count"].between?(2, 30) && rollback["promotion_approval_required"] == true
fail_contract.call("manifest rollback metadata") unless manifest.dig("rollback", "strategy") == rollback["strategy"] && manifest.dig("rollback", "retention_count") == rollback["retention_count"] && manifest.dig("rollback", "approval_required") == true

packages = spec.fetch("packages")
fail_contract.call("package provenance list") unless packages.is_a?(Array) && packages.length.between?(1, 64)
names = packages.map { |package| package["name"] }
fail_contract.call("package names") unless names.uniq.length == names.length && names.all? { |name| name.match?(/\A[a-z0-9][a-z0-9+.-]*\z/) }
packages.each do |package|
  fail_contract.call("package #{package["name"]} provenance") unless package["source"] == "amazon-linux-2023-dnf" && package["version_policy"] == "pinned-by-image-build" && package["provenance"] == "rpm-query"
end

fail_contract.call("Packer source") unless hcl.include?(%q[source "amazon-ebs" "amazon_linux_x86_64"])
fail_contract.call("moving source alias") if hcl.match?(/source_ami_filter|most_recent|owners\s*=|amazon\/linux|resolve\s*=|latest/i)
install_script = File.join(File.dirname(hcl_path), "scripts/install-base-tools.sh")
fail_contract.call("runner installation in base image") if hcl.match?(/actions-runner|config\.sh|run\.sh/i) || File.read(install_script).match?(/actions-runner|config\.sh|run\.sh/i)
fail_contract.call("root execution policy") unless hcl.include?("execute_command   = \"sudo bash")

puts "AMI contract validation passed: #{metadata.fetch("name")}@#{version}"
RUBY

if [ "$real_packer" -eq 1 ]; then
  command -v packer >/dev/null 2>&1 || { echo 'packer is required for --real-packer' >&2; exit 127; }
  (cd "$repo_root/ami" && packer fmt -check . && packer init . && packer validate -var-file="$var_file" runner.pkr.hcl)
  if [ "$build" -eq 1 ]; then
    (cd "$repo_root/ami" && packer build -var-file="$var_file" runner.pkr.hcl)
  fi
fi
