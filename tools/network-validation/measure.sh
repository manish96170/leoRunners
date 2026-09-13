#!/usr/bin/env bash
set -euo pipefail
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
matrix="$root/network/reachability-matrix.v1.json"
output=""
mode=""
allow_live=false
confirm=""
usage() { printf '%s\n' 'usage: measure.sh --allow-live --confirm I_UNDERSTAND_NETWORK_MEASUREMENT [--matrix PATH] [--mode MODE] [--output PATH]'; }
while (($#)); do
  case "$1" in
    --matrix) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; matrix=$2; shift 2 ;;
    --mode) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; mode=$2; shift 2 ;;
    --output) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; output=$2; shift 2 ;;
    --allow-live) allow_live=true; shift ;;
    --confirm) [[ $# -ge 2 ]] || { usage >&2; exit 2; }; confirm=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'unknown option: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done
[[ "$allow_live" == true && "$confirm" == I_UNDERSTAND_NETWORK_MEASUREMENT ]] || { echo 'measurement is disabled; provide the explicit confirmation flag' >&2; exit 2; }
[[ -f "$matrix" ]] || { printf 'matrix not found: %s\n' "$matrix" >&2; exit 2; }
command -v ruby >/dev/null 2>&1 || { echo 'ruby is required' >&2; exit 2; }
if [[ -z "$mode" ]]; then
  mode=$(ruby -rjson -e 'puts JSON.parse(File.read(ARGV.fetch(0))).dig("spec", "egress", "mode")' "$matrix")
fi
case "$mode" in nat|vpc-endpoint|hybrid|approved-proxy) ;; *) echo 'invalid egress mode' >&2; exit 2 ;; esac
"$root/tools/network-validation/validate.sh" --matrix "$matrix" >/dev/null
cost_labels=$(ruby -rjson -e 'doc = JSON.parse(File.read(ARGV.fetch(0))); puts JSON.generate(doc.dig("spec", "cost", "labels", ARGV.fetch(1)) || [])' "$matrix" "$mode")

tmp=$(mktemp "${TMPDIR:-/tmp}/leo-network-measure.XXXXXX")
trap 'rm -f "$tmp"' EXIT
printf '%s\n' '{"status":"COMPLETE","mode":"'"$mode"'","costLabels":'"$cost_labels"',"mutations":false,"checks":[' > "$tmp"
first=true
required_failed=false
while IFS=$'\t' read -r id host port path required; do
  [[ "$first" == true ]] || printf ',\n' >> "$tmp"
  first=false
  dns=FAIL
  connect=FAIL
  start=$(date +%s)
  if (command -v getent >/dev/null 2>&1 && getent ahostsv4 "$host" >/dev/null 2>&1) || (command -v dig >/dev/null 2>&1 && dig +time=2 +tries=1 +short "$host" | grep -q .); then dns=PASS; fi
  if command -v curl >/dev/null 2>&1 && curl --silent --show-error --output /dev/null --connect-timeout 5 --max-time 15 --proto '=https' --tlsv1.2 "https://${host}:${port}${path}" >/dev/null 2>&1; then connect=PASS; fi
  elapsed=$(( $(date +%s) - start ))
  ruby -rjson -e 'puts JSON.generate({"id"=>ARGV[0],"hostname"=>ARGV[1],"dns"=>ARGV[2],"connect"=>ARGV[3],"latencyMs"=>ARGV[4].to_i * 1000,"required"=>ARGV[5] == "true"})' "$id" "$host" "$dns" "$connect" "$elapsed" "$required" >> "$tmp"
  if [[ "$required" == true && ( "$dns" != PASS || "$connect" != PASS ) ]]; then required_failed=true; fi
done < <(ruby -rjson -e 'JSON.parse(File.read(ARGV.fetch(0))).fetch("spec").fetch("destinations").each { |d| puts [d.fetch("id"), d.fetch("hostname"), d.fetch("port"), d.fetch("path"), d.fetch("required", false)].join("\t") }' "$matrix")
printf '%s\n' ']}' >> "$tmp"
if [[ -n "$output" ]]; then
  mkdir -p "$(dirname -- "$output")"
  (umask 077; mv "$tmp" "$output")
  trap - EXIT
  printf 'measurement report written: %s\n' "$output"
else
  cat "$tmp"
fi
if [[ "$required_failed" == true ]]; then
  exit 1
fi
