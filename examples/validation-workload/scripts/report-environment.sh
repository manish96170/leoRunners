#!/usr/bin/env bash
# Prints what this runner is, and writes the same to the job summary when run in
# GitHub Actions. It never fails: a missing tool is reported as "not installed",
# because the point is to see what the runner image provides.
set -u

line() { printf '%s\n' "$*"; if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then printf '%s\n' "$*" >> "$GITHUB_STEP_SUMMARY"; fi; }
ver() { # name command...
  local name=$1; shift
  if command -v "$1" >/dev/null 2>&1; then line "| $name | $("$@" 2>&1 | head -1) |"; else line "| $name | not installed |"; fi
}

line "### Runner environment"
line ""
line "| Item | Value |"
line "|---|---|"
line "| runner name | ${RUNNER_NAME:-unknown} |"
line "| runner OS / arch | ${RUNNER_OS:-unknown} / ${RUNNER_ARCH:-unknown} |"
line "| kernel | $(uname -sm) |"
line "| CPUs | $(getconf _NPROCESSORS_ONLN 2>/dev/null || echo unknown) |"
line "| disk free (workspace) | $(df -h "${GITHUB_WORKSPACE:-.}" 2>/dev/null | awk 'NR==2 {print $4}') |"
ver node node --version
ver yarn yarn --version
ver git git --version
ver docker docker --version
ver "google-chrome" google-chrome --version
ver chromium chromium --version
ver terraform terraform version
line ""
