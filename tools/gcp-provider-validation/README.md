# GCP Provider Validation

This offline gate validates the Compute Engine runner-provider contract:
project and zone scope, pinned instance templates, ownership labels, service
account and network placement, request identity, readiness states, idempotent
deletion, bounded timeouts, and redaction.

    ./validate.sh
    ./test.sh

The validator reads JSON fixtures only. It does not load Google credentials,
initialize clients, or make network calls. `real-gcp.sh` is a separate,
explicitly guarded procedure and remains a non-mutating scaffold that exits
before any `gcloud` command.
