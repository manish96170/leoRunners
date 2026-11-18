# Production Preflight Handoff

validate.sh validates a reviewed activation request and runs the existing
cloud-validation harness in read-only preflight mode. It is a handoff gate,
not an activation command.

## Contract

The request is strict, non-secret key=value text:

~~~text
request_version=production-preflight.v1
mode=preflight
provider=all
live_action=none
allow_live=false
confirmation=none
owner=release-review
~~~

The reviewed scope file is mandatory. The wrapper passes only --provider,
--scope-file, and a temporary --report to tools/cloud-validation/validate.sh.
It never forwards --allow-live, --live-action, --confirm, or --evidence-report,
and it never performs cloud mutation itself.

## Usage

~~~sh
tools/production-preflight/validate.sh \
  --request validation/production-preflight-request.env \
  --scope-file validation/approved-scope.env \
  --report /tmp/leo-production-preflight.json
~~~

The command prints the redacted cloud-validation transcript and finishes with
PASS production-preflight or BLOCKED production-preflight. Exit code 0 means
the request and read-only provider preflight passed; exit code 1 means the
handoff is blocked. Malformed command-line usage exits 2.

The optional report is written atomically with owner-only permissions. It
contains the redacted cloud report, a bounded redacted transcript, request
status, and explicit mutation-guard fields. Credentials, tokens, reviewed
scope values, and provider response bodies are withheld.

## Tests

~~~sh
tools/production-preflight/test.sh
~~~

Tests use the existing cloud-validation fixtures and do not contact AWS, GCP,
or GitHub.
