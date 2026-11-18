# Controlled Workload Validation

This runbook turns an approved workload intake into a bounded execution and
benchmark handoff. It complements `docs/workload-intake.md`, the workload
manifest, cloud-validation evidence, and benchmark validation procedures.
The default path is read-only with respect to cloud resources and GitHub
runner registration.

## Preconditions and fixed commit

Require a request ID, workload owner, reviewer, rollback owner, escalation
contact, retention deadline, approved repository and workflow scope, full
commit ID, manifest digest, scope file, and execution deadline. Recompute the
manifest SHA-256 digest immediately before execution and compare it with the
review record. A changed ref, dirty checkout, changed manifest byte, missing
review, or expired deadline is `BLOCKED`.

The checkout must contain only the approved commit and bounded source paths.
Do not follow a moving branch, pull new workflow content during execution,
or expand intake because a command requests another file. A fixed commit is
an identity and reproducibility requirement; it is not permission to run all
commands found in that commit.

## Command review

Build a command inventory before starting. For each command or script entry
point, record its repository-relative source, exact invocation, working
directory, declared inputs, outputs, toolchain, expected exit status, timeout,
and whether it can access the network or services. Review shell expansion,
child processes, package-install steps, container execution, artifact upload,
and cleanup commands as part of the same inventory.

Reject or return for review any command that:

- is assembled from untrusted or unbounded input;
- reads credentials, host files, metadata endpoints, or undeclared mounts;
- downloads executable content without a pinned source and digest;
- changes its own limits, isolation profile, or evidence destination;
- writes outside the approved workspace or artifact paths;
- contacts an undeclared host, uses an undeclared protocol, or opens inbound
  listeners; or
- emits secrets, unrestricted payloads, or output that cannot be bounded.

Command review is evidence, not a one-time assertion. Compare the command
inventory with the actual process and network observations after execution.
Unexpected behavior invalidates the run even when the command exits zero.

## Execution limits

Set and record hard limits before starting:

- overall validation deadline and per-command timeout;
- maximum output bytes, lines/events, and retained log size;
- CPU, memory, disk, process-count, file-count, and artifact-size limits;
- maximum concurrency and child-process depth; and
- maximum retries, package downloads, and network response size.

Use bounded cancellation that terminates the process tree and waits for
termination. An output or resource limit is a failure condition, not a reason
to truncate silently and report success. Keep only redacted, bounded output
summaries and content digests in the evidence package.

## Network isolation

Start with no network access unless the reviewed manifest requires it. When
access is required, use an isolated profile with an explicit allowlist of
hostnames or service identities, ports, protocols, DNS behavior, and response
limits. Deny inbound traffic, host networking, metadata services, arbitrary
egress, and undeclared proxies. Record the profile digest and observed
destinations without storing tokens, headers, request bodies, or full
responses.

Network access must not be inferred from a successful command. A denied
connection is a review result, and a successful connection to an undeclared
destination is an abort condition. Any network exception requires a new
manifest or an explicit reviewed change before another run.

## Redaction and evidence

Redact before persistence and before handing evidence to another system.
Reject access keys, bearer tokens, private keys, webhook secrets, passwords,
raw environment values, signed URLs, customer data, full workflow payloads,
and unrestricted command output. Use stable redacted identifiers and hashes
when correlation is needed. Write evidence atomically with owner-only access
and bounded retention.

Record at least:

- request, repository, full commit, manifest digest, and scope identity;
- command-review version and execution-profile digest;
- start, finish, timeout, output-limit, network, and cleanup checkpoints;
- toolchain, architecture, cache mode/state, resources, units, and samples;
- artifact digests and bounded references;
- synthetic, mock, or real provenance for every measurement; and
- final status, reviewer, rollback owner, and retention deadline.

Use `PASS`, `WARN`, `FAIL`, `BLOCKED`, or `INCOMPLETE` explicitly. A missing
checkpoint, redaction failure, invalid digest, incomplete cleanup, or
unavailable provider is never silently converted into `PASS`.

## Abort, cleanup, and rollback

Abort for scope drift, command mismatch, secret detection, unexpected
network access, host or metadata access, privilege escalation, deadline or
resource breach, output flood, or an owner/reviewer mismatch. Cancellation and
operator interruption use the same cleanup path as a failed command.

Cleanup must terminate the entire process tree, remove temporary checkouts
and credentials, close network and service handles, collect bounded failure
evidence, release reservations, and verify that no runner or cloud resource
remains. Do not hand off a benchmark result when cleanup is unproven. Record
the prior approved manifest/profile and restore it when a failed rollout
changes local validation configuration; never edit a historical manifest in
place.

## Benchmark handoff

Before handoff, compare the run with the approved baseline across commit,
manifest, command, toolchain, architecture, provider, region or zone, cache
state, network profile, resource limits, sample count, units, and pricing
assumptions. Mark changed or missing dimensions as non-comparable or
`WARN`/`BLOCKED` according to the approved gate. Do not update a baseline from
a run with incomplete cleanup, synthetic data presented as real, or an
unexplained command or environment change.

The handoff should contain only the immutable manifest and digest, redacted
evidence references, bounded measurement summaries, comparability decision,
cleanup proof, and approval records. It is an input to benchmark review, not
authorization to provision infrastructure or register a runner.

## Later AWS and GitHub boundaries

AWS workload execution requires a later, separately approved validation that
checks account and region scope, IAM and launch-template contracts, network
isolation, cost limits, ephemeral naming, readiness deadlines, and cleanup
discovery. GitHub workload execution requires separate organization,
repository, event, labels, runner-group, fork policy, and single-use JIT
approval, followed by registration and cleanup evidence.

Until those gates pass, use local or mock execution only. A successful Phase
61 run does not grant cloud credentials, invoke provider APIs, register a
GitHub runner, execute an arbitrary workflow, or alter production scheduling.
