#!/usr/bin/env bash
# Extension for front-end workloads: a pinned Chrome, the libraries Cypress needs
# to drive it, Docker, and optionally a pinned Terraform. Pass it to the AMI build
# as `extension_script`; the versions come from `extension_environment`, so the
# choices live with the customer and not here.
#
#   CHROME_RPM_URL       https URL of one google-chrome-stable RPM (required)
#   CHROME_RPM_SHA256    its SHA-256 (required)
#   INSTALL_DOCKER       "true" (default) or "false"
#   TERRAFORM_VERSION    optional; with TERRAFORM_SHA256_LINUX_AMD64, installs
#   TERRAFORM_SHA256_LINUX_AMD64   /usr/local/bin/terraform for linux/amd64
#   CHROME_LAUNCH_CHECK  "true" (default) starts Chrome headless once and fails the
#                        build if it cannot. Set "false" only when building under
#                        CPU emulation, where Chrome cannot start at all.
#
# Google keeps only the current Chrome in its repository, so a version URL stops
# working within weeks. Download the RPM once, check it, keep it in storage you
# control, and point CHROME_RPM_URL there: the build then stays reproducible.
#
# Runs as root on Amazon Linux 2023 (x86_64). Node.js is not installed: a
# workflow's actions/setup-node fetches the exact version it asks for.
set -Eeuo pipefail

fail() { printf 'frontend-browser extension: %s\n' "$*" >&2; exit 1; }

: "${CHROME_RPM_URL:?CHROME_RPM_URL is required}"
: "${CHROME_RPM_SHA256:?CHROME_RPM_SHA256 is required}"
INSTALL_DOCKER=${INSTALL_DOCKER:-true}
CHROME_LAUNCH_CHECK=${CHROME_LAUNCH_CHECK:-true}
TERRAFORM_VERSION=${TERRAFORM_VERSION:-}
TERRAFORM_SHA256_LINUX_AMD64=${TERRAFORM_SHA256_LINUX_AMD64:-}

[[ "$CHROME_RPM_URL" =~ ^https://[A-Za-z0-9._~:/?#@!$\&\'\(\)*+,\;=%-]+$ ]] || fail "CHROME_RPM_URL must be an https URL"
[[ "$CHROME_RPM_SHA256" =~ ^[0-9a-f]{64}$ ]] || fail "CHROME_RPM_SHA256 must be 64 lower-case hex characters"
case "$INSTALL_DOCKER" in true | false) ;; *) fail "INSTALL_DOCKER must be true or false" ;; esac
case "$CHROME_LAUNCH_CHECK" in true | false) ;; *) fail "CHROME_LAUNCH_CHECK must be true or false" ;; esac
if [ -n "$TERRAFORM_VERSION$TERRAFORM_SHA256_LINUX_AMD64" ]; then
  [[ "$TERRAFORM_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "TERRAFORM_VERSION must look like 1.9.8"
  [[ "$TERRAFORM_SHA256_LINUX_AMD64" =~ ^[0-9a-f]{64}$ ]] || fail "TERRAFORM_SHA256_LINUX_AMD64 must be 64 lower-case hex characters"
fi
[ "$(uname -m)" = x86_64 ] || fail "this extension is for x86_64 images (found $(uname -m))"
[ "${EUID}" -eq 0 ] || fail "must run as root"
command -v dnf >/dev/null 2>&1 || fail "Amazon Linux 2023 dnf not found"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Libraries Cypress needs to start a browser under a virtual display, then Chrome
# itself, installed from the verified file so that dnf resolves its dependencies.
dnf install -y xorg-x11-server-Xvfb gtk3 nss alsa-lib libXScrnSaver libXtst at-spi2-atk
curl -fsSL --retry 5 --retry-connrefused --retry-delay 2 -o "$work/chrome.rpm" "$CHROME_RPM_URL"
printf '%s  %s\n' "$CHROME_RPM_SHA256" "$work/chrome.rpm" | sha256sum -c - >/dev/null \
  || fail "Chrome RPM checksum mismatch; refusing to install it"
dnf install -y "$work/chrome.rpm"
chrome_version=$(google-chrome --version)
if [ "$CHROME_LAUNCH_CHECK" = true ]; then
  # Installing the package is not proof it runs: a missing library shows up only
  # when the browser starts. As root, Chrome needs --no-sandbox for this one check.
  dom=$(google-chrome --headless=new --no-sandbox --disable-gpu --dump-dom 'data:text/html,<title>leo-runners</title>' 2>/dev/null) \
    || fail "Chrome is installed but did not start"
  grep -q 'leo-runners' <<<"$dom" || fail "Chrome started but did not render a page"
fi

docker_version=none
if [ "$INSTALL_DOCKER" = true ]; then
  dnf install -y docker
  systemctl enable docker
  getent group docker >/dev/null || fail "the docker package did not create the docker group"
  docker_version=$(docker --version)
fi

terraform_version=none
if [ -n "$TERRAFORM_VERSION" ]; then
  dnf install -y unzip
  archive="terraform_${TERRAFORM_VERSION}_linux_amd64.zip"
  curl -fsSL --retry 5 --retry-connrefused --retry-delay 2 -o "$work/$archive" "https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/${archive}"
  printf '%s  %s\n' "$TERRAFORM_SHA256_LINUX_AMD64" "$work/$archive" | sha256sum -c - >/dev/null \
    || fail "Terraform archive checksum mismatch; refusing to install it"
  unzip -o -q "$work/$archive" -d "$work/tf"
  install -m 0755 "$work/tf/terraform" /usr/local/bin/terraform
  # sed, not head: under pipefail, head closing the pipe early kills terraform
  # with SIGPIPE and fails the whole build.
  terraform_version=$(/usr/local/bin/terraform version | sed -n 1p)
fi

dnf clean all
rm -rf /var/cache/dnf

# What this image carries, for tracing an AMI back to what is in it.
install -d -m 0755 /etc/leo-runners
{
  printf 'extension=frontend-browser\n'
  printf 'chrome=%s\n' "$chrome_version"
  printf 'docker=%s\n' "$docker_version"
  printf 'terraform=%s\n' "$terraform_version"
} > /etc/leo-runners/extension-frontend-browser
chmod 0644 /etc/leo-runners/extension-frontend-browser

# Print the record so the build log shows exactly what this image carries.
printf 'frontend-browser extension installed:\n'
sed 's/^/  /' /etc/leo-runners/extension-frontend-browser

# Leave nothing a job could trip over or an auditor could question.
rm -f /root/.bash_history
