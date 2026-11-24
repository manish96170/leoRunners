#!/usr/bin/env bash
# Read-only check of one launched runner instance against the runner contract.
#
#   check.sh --instance-id i-... [--region R] [--request-id ID] [--tag K=V]...
#            [--profile-arn ARN] [--allow-state running|pending|any]
#
# It only calls ec2 describe-* and never changes anything. It prints one line per
# check and exits non-zero if any fails, so its output can be kept as evidence of
# the first real launch. The one check it cannot make from outside, that IMDSv1 is
# actually refused, is printed as a command to run on the instance.
set -euo pipefail

instance_id= region= request_id= profile_arn= allow_state=any
tags=()
while [ $# -gt 0 ]; do
  case "$1" in
    --instance-id) instance_id=${2:?}; shift ;;
    --region) region=${2:?}; shift ;;
    --request-id) request_id=${2:?}; shift ;;
    --tag) tags+=("${2:?}"); shift ;;
    --profile-arn) profile_arn=${2:?}; shift ;;
    --allow-state) allow_state=${2:?}; shift ;;
    -h | --help) sed -n 2,11p "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
[[ "$instance_id" =~ ^i-[0-9a-f]{8}([0-9a-f]{9})?$ ]] || { echo "--instance-id must look like i-0123456789abcdef0" >&2; exit 2; }
command -v aws >/dev/null || { echo "aws CLI not found" >&2; exit 2; }
command -v python3 >/dev/null || { echo "python3 not found" >&2; exit 2; }

region_args=()
[ -n "$region" ] && region_args=(--region "$region")
ec2() { aws "${region_args[@]+"${region_args[@]}"}" ec2 "$@" --output json; }

instance_json=$(ec2 describe-instances --instance-ids "$instance_id")
shutdown_json=$(ec2 describe-instance-attribute --instance-id "$instance_id" --attribute instanceInitiatedShutdownBehavior)
volume_ids=$(python3 - "$instance_json" <<'PY'
import json, sys
inst = json.loads(sys.argv[1])["Reservations"][0]["Instances"][0]
print(" ".join(m["Ebs"]["VolumeId"] for m in inst.get("BlockDeviceMappings", []) if "Ebs" in m))
PY
)
volumes_json='{"Volumes":[]}'
# shellcheck disable=SC2086
[ -n "$volume_ids" ] && volumes_json=$(ec2 describe-volumes --volume-ids $volume_ids)

python3 - "$instance_json" "$shutdown_json" "$volumes_json" "$request_id" "$profile_arn" "$allow_state" "${tags[@]+"${tags[@]}"}" <<'PY'
import json, sys

instance = json.loads(sys.argv[1])["Reservations"][0]["Instances"][0]
shutdown = json.loads(sys.argv[2])
volumes = json.loads(sys.argv[3])["Volumes"]
request_id, profile_arn, allow_state = sys.argv[4], sys.argv[5], sys.argv[6]
extra = dict(t.split("=", 1) for t in sys.argv[7:] if "=" in t)
if request_id:
    extra["RequestId"] = request_id

failures = 0
def check(ok, name, detail=""):
    global failures
    print(("PASS  " if ok else "FAIL  ") + name + (f"  ({detail})" if detail else ""))
    if not ok:
        failures += 1

tag_of = lambda resource: {t["Key"]: t["Value"] for t in resource.get("Tags", [])}
tags = tag_of(instance)

state = instance["State"]["Name"]
want = {"running": ["running"], "pending": ["pending"], "any": ["pending", "running"]}.get(allow_state, ["pending", "running"])
check(state in want, "instance is live", state)

interfaces = instance.get("NetworkInterfaces", [])
public = [i for i in interfaces if i.get("Association", {}).get("PublicIp")]
check(not instance.get("PublicIpAddress") and not public, "no public IP", instance.get("PublicIpAddress") or "none")
check(len(interfaces) == 1 and interfaces[0].get("Attachment", {}).get("DeleteOnTermination") is True,
      "one network interface, deleted with the instance", f"{len(interfaces)} interface(s)")

md = instance.get("MetadataOptions", {})
check(md.get("HttpTokens") == "required", "IMDSv2 required (IMDSv1 disabled in configuration)", md.get("HttpTokens", "unset"))
check(md.get("HttpPutResponseHopLimit") == 1, "IMDS hop limit is 1", str(md.get("HttpPutResponseHopLimit")))
check(md.get("InstanceMetadataTags", "disabled") == "disabled", "instance metadata tags disabled", md.get("InstanceMetadataTags", "disabled"))

check(shutdown.get("InstanceInitiatedShutdownBehavior", {}).get("Value") == "terminate", "shutdown terminates the instance",
      shutdown.get("InstanceInitiatedShutdownBehavior", {}).get("Value", "unset"))

check(tags.get("leo-runners:managed-by") == "leo-runners", "instance carries leo-runners:managed-by", tags.get("leo-runners:managed-by", "missing"))
for key in ("leo-runners:runner-id", "leo-runners:expires-at"):
    check(bool(tags.get(key)), f"instance carries {key}", tags.get(key, "missing"))
for key, value in sorted(extra.items()):
    check(tags.get(key) == value, f"instance carries {key}={value}", tags.get(key, "missing"))

profile = instance.get("IamInstanceProfile", {}).get("Arn", "")
check(bool(profile), "instance has an instance profile", profile or "none")
if profile_arn:
    check(profile == profile_arn, "instance profile is the expected one", profile)

check(len(volumes) > 0, "instance has attached volumes", f"{len(volumes)} volume(s)")
for v in volumes:
    vt = tag_of(v)
    vid = v["VolumeId"]
    attachment = (v.get("Attachments") or [{}])[0]
    check(v.get("Encrypted") is True, f"{vid} is encrypted")
    check(v.get("VolumeType") == "gp3", f"{vid} is gp3", v.get("VolumeType", "unset"))
    check(attachment.get("DeleteOnTermination") is True, f"{vid} is deleted with the instance")
    check(vt.get("leo-runners:managed-by") == "leo-runners", f"{vid} carries leo-runners:managed-by", vt.get("leo-runners:managed-by", "missing"))
    for key, value in sorted(extra.items()):
        check(vt.get(key) == value, f"{vid} carries {key}={value}", vt.get(key, "missing"))

template = tags.get("aws:ec2launchtemplate:id")
print(f"INFO  launched from template {template}" if template else "INFO  no launch template tag found")

print()
print("Not checkable from outside: that IMDSv1 is refused at runtime. On the instance (for example")
print("through Session Manager) this must print 401, and must succeed only with a token:")
print("  curl -s -o /dev/null -w '%{http_code}\\n' http://169.254.169.254/latest/meta-data/instance-id")
print()
print("RESULT: " + ("all checks passed" if failures == 0 else f"{failures} check(s) failed"))
sys.exit(1 if failures else 0)
PY
