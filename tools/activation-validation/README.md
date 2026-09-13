# Activation request validation

This offline validator checks the versioned activation-request contract before
any provider operation is considered. It requires separate operator,
approver, and rollback owners; an explicit AWS, GCP, or GitHub target; a
SHA-256 scope-file hash; bounded resources and budget; an expiry deadline;
notification routing; and one of `read-only`, `connectivity`, or `lifecycle`
mode.

Lifecycle requests additionally require confirmation metadata with the exact
ephemeral-resource acknowledgement phrase and a change reference. Secrets,
wildcard scopes, invalid or expired deadlines, missing owners, and unsupported
fields fail closed.

Run `./test.sh` for deterministic fixture coverage. Set
`LEO_ACTIVATION_NOW` to an RFC3339 timestamp when validating expiry behavior in
repeatable automation. The validator only reads local JSON and never invokes a
cloud CLI, SDK, Terraform, or Kubernetes command.
