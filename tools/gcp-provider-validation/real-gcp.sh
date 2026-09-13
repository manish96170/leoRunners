#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf '%s\n' 'Usage: real-gcp.sh --allow-live --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES'
  printf '%s\n' 'Guarded procedure only; this scaffold never executes gcloud commands.'
}
allow_live=false
confirmation=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    --allow-live) allow_live=true ;;
    --confirm) [ "$#" -ge 2 ] || { echo '--confirm requires a value' >&2; exit 2; }; confirmation=$2; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done
if [ "$allow_live" != true ] || [ "$confirmation" != I_UNDERSTAND_EPHEMERAL_RESOURCES ]; then
  echo 'GCP live validation is disabled; explicit confirmation is required.' >&2
  exit 1
fi
echo 'GCP live validation scaffold is intentionally unavailable until an approved implementation is supplied.' >&2
exit 1
