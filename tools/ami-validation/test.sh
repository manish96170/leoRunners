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

printf '%s\n' 'AMI validation tests passed'
