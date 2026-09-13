#!/usr/bin/env bash
set -Eeuo pipefail
umask 022

if [[ "${EUID}" -ne 0 ]]; then
  printf '%s\n' 'install-base-tools.sh must run as root' >&2
  exit 1
fi

command -v dnf >/dev/null 2>&1 || {
  printf '%s\n' 'Amazon Linux 2023 dnf was not found' >&2
  exit 1
}

# Keep the image focused on tools required to start and execute a runner job.
# Runner registration, JIT configuration, credentials, and workload-specific
# dependencies are intentionally supplied at runtime or by the workflow.
dnf upgrade -y --refresh
dnf install -y \
  ca-certificates \
  curl-minimal \
  git \
  gzip \
  jq \
  make \
  openssl \
  tar \
  unzip \
  which

dnf clean all
rm -rf /var/cache/dnf

install -d -m 0755 /etc/leo-runners
printf '%s\n' 'amazon-linux-2023-base-tools' > /etc/leo-runners/image-profile
chmod 0644 /etc/leo-runners/image-profile

# Do not leave build-time SSH material, shell history, package metadata, or
# temporary credentials in the image snapshot.
rm -f /root/.bash_history /home/ec2-user/.bash_history
rm -rf /tmp/* /var/tmp/*
