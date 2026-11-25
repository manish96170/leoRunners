#!/usr/bin/env bash
set -Eeuo pipefail

script_dir=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../.." && pwd)

"$script_dir/validate.sh"

if "$script_dir/validate.sh" --real-packer --var-file "$repo_root/ami/example.pkrvars.hcl" --confirm WRONG_TOKEN >/dev/null 2>&1; then
  printf '%s\n' 'real Packer path accepted an incorrect confirmation token' >&2
  exit 1
fi

if "$script_dir/validate.sh" --build --confirm I_UNDERSTAND_PACKER_BUILD --var-file "$repo_root/ami/example.pkrvars.hcl" >/dev/null 2>&1; then
  printf '%s\n' '--build unexpectedly bypassed --real-packer' >&2
  exit 1
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/leo-ami-validation.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT

mkdir -p "$tmp_dir/bin"
cat >"$tmp_dir/bin/packer" <<'PACKER'
#!/usr/bin/env sh
touch "${AMI_TEST_PACKER_MARKER:?}"
exit 99
PACKER
chmod +x "$tmp_dir/bin/packer"
marker="$tmp_dir/packer-invoked"
AMI_TEST_PACKER_MARKER="$marker" PATH="$tmp_dir/bin:$PATH" "$script_dir/validate.sh" >/dev/null
if [ -e "$marker" ]; then
  printf '%s\n' 'offline validation invoked Packer' >&2
  exit 1
fi

cp "$repo_root/ami/image-contract.v1.json" "$tmp_dir/unsafe.json"
ruby -rjson - "$tmp_dir/unsafe.json" <<'RUBY'
path = ARGV.fetch(0)
data = JSON.parse(File.read(path))
data["spec"]["source"]["ami_id"] = "latest-amazon-linux"
File.write(path, JSON.pretty_generate(data) + "\n")
RUBY
if "$script_dir/validate.sh" --contract "$tmp_dir/unsafe.json" >/dev/null 2>&1; then
  printf '%s\n' 'moving source fixture unexpectedly passed' >&2
  exit 1
fi

# Derived profiles are held to the same rules as the base. Each mutation below
# works on a private copy of the repository's ami/ and validator and must fail.
derived_fails() { # description sed-or-ruby-expression-file
  local name=$1 expression=$2
  local copy="$tmp_dir/copy-$name"
  mkdir -p "$copy/tools" "$copy/ami"
  cp -R "$repo_root/ami/." "$copy/ami/"
  cp -R "$repo_root/tools/ami-validation" "$copy/tools/ami-validation"
  ruby -ryaml - "$copy/ami/profiles/frontend-browser-linux-x64.yaml" "$expression" <<'RUBY'
path, mutation = ARGV
data = YAML.safe_load(File.read(path))
case mutation
when "weaker-security" then data["spec"]["security"]["requireIMDSv2"] = false
when "wrong-parent" then data["spec"]["parent"]["name"] = "something-else"
when "user-data-installs" then data["spec"]["bootstrap"]["userDataMayInstallPackages"] = true
when "baked-runner" then data["spec"]["tools"] << {"name" => "github-actions-runner", "version" => "pinned-by-build-input"}
when "floating-tool" then data["spec"]["tools"][0]["version"] = "latest"
when "missing-extension" then data["spec"]["extension"]["script"] = "ami/extensions/does-not-exist.sh"
when "wrong-architecture" then data["spec"]["architecture"] = "arm64"
else abort "unknown mutation #{mutation}"
end
File.write(path, YAML.dump(data))
RUBY
  if "$copy/tools/ami-validation/validate.sh" >/dev/null 2>&1; then
    printf 'derived profile mutation %s unexpectedly passed\n' "$name" >&2
    exit 1
  fi
}
# Control: an unmutated copy must pass, so a failure below means the mutation.
control="$tmp_dir/copy-control"
mkdir -p "$control/tools" "$control/ami"
cp -R "$repo_root/ami/." "$control/ami/"
cp -R "$repo_root/tools/ami-validation" "$control/tools/ami-validation"
"$control/tools/ami-validation/validate.sh" >/dev/null 2>&1 || { printf '%s\n' 'unmutated copy of the repository failed validation' >&2; exit 1; }
for mutation in weaker-security wrong-parent user-data-installs baked-runner floating-tool missing-extension wrong-architecture; do
  derived_fails "$mutation" "$mutation"
done

printf '%s\n' 'AMI validation tests passed'
