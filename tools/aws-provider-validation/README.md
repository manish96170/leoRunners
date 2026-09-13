# AWS Provider Validation

This offline gate validates the EC2 runner-provider contract: pinned Launch
Template input, IMDSv2, ownership tags, instance profile and network
placement, idempotency, readiness, bounded timeouts, and redaction.

    ./validate.sh
    ./test.sh

The validator reads JSON fixtures only. It does not load AWS credentials,
initialize SDK clients, or make network calls. real-ec2.sh is a separate,
explicitly guarded procedure and exits before any AWS command.
