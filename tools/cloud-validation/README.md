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
syntax, missing-variable failures, the exact live confirmation guard,
read-only non-creation, AWS cleanup, and secret-free JSON reporting. They do
not contact AWS, GCP, or GitHub.

Live validation is still an operational gate, not proof that a complete GitHub
job ran. Use a dedicated account/project, a validation-only Launch Template
and instance template, least-privilege identities, and a budget alert before
running it.
