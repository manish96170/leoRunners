#!/usr/bin/env bash
# Offline test for check.sh: a stub `aws` serves fixtures, so nothing touches AWS.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

cat > "$work/bin/aws" <<'STUB'
#!/usr/bin/env bash
args=" $* "
case "$args" in
  *" describe-instance-attribute "*) cat "$FIXTURES/shutdown.json" ;;
  *" describe-volumes "*) cat "$FIXTURES/volumes.json" ;;
  *" describe-instances "*) cat "$FIXTURES/instance.json" ;;
  *) echo "unexpected aws call: $*" >&2; exit 99 ;;
esac
STUB
chmod +x "$work/bin/aws"

export PATH="$work/bin:$PATH" FIXTURES="$work/fx"
fail() { printf 'launch-check test FAILED: %s\n' "$*" >&2; exit 1; }

reset() {
  rm -rf "$FIXTURES"; mkdir -p "$FIXTURES"
  cp "$here/fixtures/good-instance.json" "$FIXTURES/instance.json"
  cp "$here/fixtures/good-volumes.json" "$FIXTURES/volumes.json"
  cp "$here/fixtures/good-shutdown.json" "$FIXTURES/shutdown.json"
}
# mutate FILE 'python statement on d'
mutate() {
  python3 - "$FIXTURES/$1" "$2" <<'PY'
import json, sys
path, stmt = sys.argv[1], sys.argv[2]
d = json.load(open(path))
exec(stmt)
json.dump(d, open(path, "w"))
PY
}
run() { "$here/check.sh" --instance-id i-0123456789abcdef0 "$@" 2>&1 || true; }
expect_pass() {
  local out; out=$("$here/check.sh" --instance-id i-0123456789abcdef0 "$@" 2>&1) || fail "expected a pass, got: $out"
  grep -q "RESULT: all checks passed" <<<"$out" || fail "no pass result: $out"
}
expect_fail() { # expected-fragment args...
  local want=$1; shift
  local out status=0
  out=$("$here/check.sh" --instance-id i-0123456789abcdef0 "$@" 2>&1) || status=$?
  [ "$status" -ne 0 ] || fail "expected a failure containing '$want', but it passed"
  grep -q "FAIL  .*$want" <<<"$out" || fail "expected FAIL for '$want', got: $out"
}

reset; expect_pass --request-id req-1
reset; expect_pass --request-id req-1 --tag RequestId=req-1 --profile-arn arn:aws:iam::111122223333:instance-profile/example-runner

reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["PublicIpAddress"]="203.0.113.10"'; expect_fail "no public IP"
reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["NetworkInterfaces"][0]["Association"]={"PublicIp":"203.0.113.10"}'; expect_fail "no public IP"
reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["MetadataOptions"]["HttpTokens"]="optional"'; expect_fail "IMDSv2 required"
reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["MetadataOptions"]["HttpPutResponseHopLimit"]=2'; expect_fail "hop limit"
reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["MetadataOptions"]["InstanceMetadataTags"]="enabled"'; expect_fail "metadata tags"
reset; mutate instance.json 'i=d["Reservations"][0]["Instances"][0]; i["Tags"]=[t for t in i["Tags"] if t["Key"]!="leo-runners:managed-by"]'; expect_fail "instance carries leo-runners:managed-by"
reset; mutate instance.json 'i=d["Reservations"][0]["Instances"][0]; i["Tags"]=[t for t in i["Tags"] if t["Key"]!="RequestId"]'; expect_fail "instance carries RequestId=req-1" --request-id req-1
reset; mutate instance.json 'd["Reservations"][0]["Instances"][0]["State"]["Name"]="terminated"'; expect_fail "instance is live"
reset; mutate instance.json 'del d["Reservations"][0]["Instances"][0]["IamInstanceProfile"]'; expect_fail "instance has an instance profile"
reset; expect_fail "instance profile is the expected one" --profile-arn arn:aws:iam::111122223333:instance-profile/other
reset; mutate shutdown.json 'd["InstanceInitiatedShutdownBehavior"]["Value"]="stop"'; expect_fail "shutdown terminates"
reset; mutate volumes.json 'd["Volumes"][0]["Encrypted"]=False'; expect_fail "is encrypted"
reset; mutate volumes.json 'd["Volumes"][0]["VolumeType"]="gp2"'; expect_fail "is gp3"
reset; mutate volumes.json 'd["Volumes"][0]["Attachments"][0]["DeleteOnTermination"]=False'; expect_fail "is deleted with the instance"
reset; mutate volumes.json 'd["Volumes"][0]["Tags"]=[]'; expect_fail "carries leo-runners:managed-by"
reset; mutate volumes.json 'd["Volumes"][0]["Tags"]=[t for t in d["Volumes"][0]["Tags"] if t["Key"]!="RequestId"]'; expect_fail "carries RequestId=req-1" --request-id req-1

# input validation
if "$here/check.sh" --instance-id nonsense >/dev/null 2>&1; then fail "a bad instance ID was accepted"; fi
if "$here/check.sh" >/dev/null 2>&1; then fail "a missing instance ID was accepted"; fi
printf '%s\n' 'launch-check tests passed'
