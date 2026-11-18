# DynamoDB state repository

This package implements `controller/internal/state.Repository` using AWS SDK for
Go v2. It does not change the controller's root module or wire itself into the
runtime.

## Table layout

The table has string partition and sort keys named `pk` and `sk`:

| Record | `pk` | `sk` | Index access |
| --- | --- | --- | --- |
| job | `job#<id>` | `META` | `gsi1pk=job#state#<state>`, `gsi2pk=expiry#job` |
| runner | `runner#<id>` | `META` | `gsi1pk=runner#state#<state>`, `gsi2pk=expiry#runner` |
| lease | `lease#<id>` | `META` | `gsi1pk=lease#state#<state>`, `gsi2pk=expiry#lease` |
| event copy | `aggregate#<id>` | `event#<timestamp>#<key>` | `gsi1pk=events` |
| event marker | `idempotency#<key>` | `META` | none |

`gsi1` is a sparse state/event index with (`gsi1pk`, `gsi1sk`) and `gsi2` is a
sparse expiry index with (`gsi2pk`, `gsi2sk`). Expiry sort keys are fixed-width
Unix seconds followed by the record ID, so reconciliation uses a bounded
`Query` with `gsi2sk <= <now>`. Listing without a state filter uses a filtered
scan because DynamoDB has no useful single-table query for all state values.

Lifecycle events are copied once for each non-empty job, lease, and runner
aggregate. A transaction writes an idempotency marker and all copies together;
the marker stores the exact copy keys for idempotent deletion. Replays compare a
stable fingerprint and return `(false, nil)` only for the same event.

## Dependency requirement

The root `controller/go.mod` was intentionally not edited. Add these direct
requirements when enabling this adapter in the controller module:

```text
github.com/aws/aws-sdk-go-v2/service/dynamodb v1.68.0
github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.21.4
```

These versions were available as the latest releases on 2026-09-13. Keep the
AWS SDK core and Smithy versions compatible with the existing controller module.
The SDK client's standard retryer handles throttling and transient transport
errors; every repository method also propagates cancellation and applies the
configured operation timeout.
