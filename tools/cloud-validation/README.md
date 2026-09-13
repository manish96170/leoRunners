# Cloud Validation Harness

Phase 15 provides a portable POSIX-shell gate for checking whether the local
machine is ready to perform the platform's real AWS, GCP, and GitHub runner
validation. It is independent of the controller and does not install SDKs or
contact cloud services in test mode.

## Default behavior

~~~sh
tools/cloud-validation/validate.sh
~~~

The default is read-only. It checks, for selected providers:

- AWS CLI, configured region, and sts get-caller-identity;
- gcloud, project selection, and Application Default Credentials;
- curl, GitHub bearer-token presence, repository shape, repository API access,
  Actions runner API access, numeric runner group, and safe labels.

Required environment variables are checked before provider calls:

~~~text
AWS_REGION (or AWS_DEFAULT_REGION)
GCP_PROJECT (or GOOGLE_CLOUD_PROJECT)
GITHUB_TOKEN (or GH_TOKEN)
GITHUB_REPOSITORY=OWNER/REPOSITORY
GITHUB_RUNNER_GROUP_ID
GITHUB_RUNNER_LABELS=comma,separated,labels
~~~

Select one provider with --provider aws, --provider gcp, or --provider github.
The default all checks every provider. Read-only mode does not call
RunInstances, TerminateInstances, GCP create/delete, or the GitHub JIT POST
endpoint.

## Reviewed scope gate

Pass `--scope-file PATH` to require the read-only preflight identity to match a
reviewed, non-secret scope file. The file is strict `key=value` text with
optional comments and supports these keys:

~~~text
aws_account_id=000000000000
aws_region=us-east-1
gcp_project=fixture-project
gcp_zone=us-central1-a
github_repository=OWNER/REPOSITORY
~~~

Each key is optional, but unknown, duplicate, malformed, or mismatched values
fail closed. AWS account and GCP project/zone are checked through read-only
identity APIs. GitHub repository scope is checked against the repository API
target after the repository and Actions APIs respond successfully. Scope
values are never included in reports, and the file must not contain tokens or
other credentials. The same gate runs before any live action; `--allow-live`
and the exact `--confirm I_UNDERSTAND_EPHEMERAL_RESOURCES` confirmation remain
required.

## Guarded live actions

Live mode requires --allow-live, an explicit --live-action, and the exact
confirmation string below. The confirmation is deliberately difficult to
provide accidentally.

~~~sh
tools/cloud-validation/validate.sh \
  --provider aws \
  --allow-live \
  --confirm I_UNDERSTAND_EPHEMERAL_RESOURCES \
  --live-action aws-ec2 \
  --report /tmp/leo-cloud-validation.json
~~~

Use `--evidence-report PATH` when a controlled-validation evidence artifact is
required. The harness emits this schema-compatible document only for one
successful AWS or GCP action after cleanup succeeds, and validates it before
atomic publication. Failed validation, incomplete cleanup, GitHub JIT, and
multi-provider actions never produce passed evidence.

Evidence identity and workflow fields can be supplied with
`LEO_EVIDENCE_RUN_ID`, `LEO_EVIDENCE_REPOSITORY`, and
`LEO_EVIDENCE_WORKFLOW`; the defaults are safe controlled-validation values.

Available actions are aws-ec2, gcp-vm, github-jit, and all.

AWS requires AWS_VALIDATION_LAUNCH_TEMPLATE_ID and optionally
AWS_VALIDATION_LAUNCH_TEMPLATE_VERSION. It launches exactly one instance from
the dedicated template with a unique client token and validation tags. The
exit trap requests termination even if the harness receives a signal or a
later check fails.

GCP requires GCP_VALIDATION_INSTANCE_TEMPLATE. It creates one uniquely named
VM from that template and the exit trap deletes it. GCP_ZONE defaults to
us-central1-a.

GitHub JIT requires the normal GitHub variables above. It makes the explicit
JIT configuration POST using a unique runner name and discards the response
body. The encoded configuration is single-use and short-lived; GitHub does not
provide a matching revoke operation, so the report records
pass-no-resource-jit-expires rather than pretending cleanup occurred.

The live report contains only fixed status fields and a redaction statement. It
never includes tokens, encoded JIT configuration, instance IDs, response
bodies, or command output. Report files are written through a restrictive
temporary file and atomic rename. Do not put reports in a committed directory.

## Tests

~~~sh
tools/cloud-validation/test.sh
~~~

The tests prepend local fixtures for aws, gcloud, and curl. They verify shell
syntax, reviewed-scope success and mismatch failures, missing-variable
failures, the exact live confirmation guard, read-only non-creation, AWS
cleanup, and secret-free JSON reporting. They do not contact AWS, GCP, or
GitHub.

Live validation is still an operational gate, not proof that a complete GitHub
job ran. Use a dedicated account/project, a validation-only Launch Template
and instance template, least-privilege identities, and a budget alert before
running it.
