# Phase 60: Approved Workload Intake

## Objective

Define a repeatable, evidence-based process for onboarding a repository into
controlled workload validation. The process captures one approved revision,
derives a bounded workload manifest, and hands the result to the existing
controlled-validation boundary. It does not execute a customer workflow,
provision runners, change an image, or enable production scheduling.

## Deliverables

- An approved repository and workflow scope.
- A fixed commit and repository identity record.
- A versioned inventory of workflows, tools, runtimes, services, and cache
  behavior.
- A redacted workload manifest with a SHA-256 digest.
- Comparability and review evidence for later validation or benchmarking.
- A rollback record that can restore the previous manifest and profile.
- A bounded handoff to controlled workload validation.

## Intake sequence

1. Record the repository owner, organization, repository, default branch, and
   approved workflow or event scope. Reject forks, unapproved repositories,
   wildcard organizations, and ambiguous ownership.
2. Resolve the requested revision to an immutable full commit ID. Capture the
   commit ID, repository URL in approved form, resolution time, and source
   reference. Do not validate a moving branch name or an unreviewed local
   working tree.
3. Inspect only the approved revision. Inventory workflow files, scripts,
   lockfiles, runtime declarations, container definitions, service
   dependencies, test commands, artifact outputs, and cache declarations.
4. Record each observed capability with a repository-relative source and
   precise locator. Distinguish `observed`, `inferred`, and `absent`; do not
   turn an assumption into a required capability.
5. Produce a versioned workload manifest. Include the fixed commit, inspection
   time, toolchain identifiers, workload commands, resource bounds, cache
   mode, and evidence references.
6. Redact and validate the manifest, calculate its SHA-256 digest, and store
   the digest with the review record. The digest covers the exact bytes that
   will be handed off.
7. Obtain an independent review of scope, evidence, comparability, security,
   and rollback. Only then hand the immutable manifest to controlled workload
   validation.

## Exit criteria

- Repository and workflow scope are explicit and approved.
- The revision is a full immutable commit and matches the review record.
- Every enabled or required capability has bounded repository evidence.
- The manifest is versioned, redacted, schema-compatible, and digest-linked.
- Toolchain, commands, resource limits, cache policy, and measurement units
  are sufficient for a later comparable run.
- Reviewer, rollback owner, retention deadline, and handoff status are
  recorded.
- No cloud mutation, runner registration, or production scheduling occurred
  during intake.

## Out of scope

Intake must not install arbitrary dependencies into a shared image, execute
unreviewed workflow code, fetch secrets, infer permissions from a successful
command, or broaden the repository, branch, event, provider, or runner-group
scope. A workload manifest is an input to validation, not authorization to
run the workload.
