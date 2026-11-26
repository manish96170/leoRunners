#!/usr/bin/env bash
# Offline checks for the extension scripts: syntax, and that bad input is refused
# before anything is installed. The real install is exercised by building an
# image (or in an Amazon Linux 2023 x86_64 container); see ../README.md.
set -Eeuo pipefail
here=$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script="$here/frontend-browser.sh"
fail() { printf 'extension test FAILED: %s\n' "$*" >&2; exit 1; }

bash -n "$script"

good_sha=d1e2336969eb47d2a72ddb503bee1bb866ef6a7ea6fc8a955bc1e9792c97a4d8
good_url=https://example.invalid/google-chrome-stable.rpm

# expect_refused DESCRIPTION MESSAGE-FRAGMENT VAR=VALUE...
expect_refused() {
  local description=$1 fragment=$2; shift 2
  local out
  if out=$(env -i PATH="$PATH" "$@" bash "$script" 2>&1); then
    fail "$description: the script ran when it should have refused"
  fi
  grep -qF -- "$fragment" <<<"$out" || fail "$description: expected \"$fragment\", got: $out"
}

expect_refused "no URL" "CHROME_RPM_URL is required" CHROME_RPM_SHA256=$good_sha
expect_refused "no checksum" "CHROME_RPM_SHA256 is required" CHROME_RPM_URL=$good_url
expect_refused "plain http" "must be an https URL" CHROME_RPM_URL=http://example.invalid/c.rpm CHROME_RPM_SHA256=$good_sha
expect_refused "file URL" "must be an https URL" CHROME_RPM_URL=file:///etc/passwd CHROME_RPM_SHA256=$good_sha
expect_refused "URL with a space" "must be an https URL" "CHROME_RPM_URL=https://example.invalid/a b.rpm" CHROME_RPM_SHA256=$good_sha
expect_refused "short checksum" "64 lower-case hex" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=abc123
expect_refused "upper-case checksum" "64 lower-case hex" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256="$(printf %s "$good_sha" | tr a-f A-F)"
expect_refused "bad docker flag" "INSTALL_DOCKER must be true or false" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha INSTALL_DOCKER=maybe
expect_refused "bad launch-check flag" "CHROME_LAUNCH_CHECK must be true or false" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha CHROME_LAUNCH_CHECK=sometimes
expect_refused "terraform without a checksum" "TERRAFORM_SHA256_LINUX_AMD64 must be 64" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha TERRAFORM_VERSION=0.12.23
expect_refused "terraform checksum without a version" "TERRAFORM_VERSION must look like" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha TERRAFORM_SHA256_LINUX_AMD64=$good_sha
expect_refused "floating terraform version" "TERRAFORM_VERSION must look like" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha TERRAFORM_VERSION=latest TERRAFORM_SHA256_LINUX_AMD64=$good_sha

# Valid input on the wrong kind of machine is refused before any install.
if [ "$(uname -m)" != x86_64 ] || [ "$(id -u)" -ne 0 ]; then
  expect_refused "wrong machine or not root" "" CHROME_RPM_URL=$good_url CHROME_RPM_SHA256=$good_sha
fi

printf '%s\n' 'extension tests passed'
