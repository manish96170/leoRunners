# Approved scope validation

`validate-scope.sh` checks an offline, credential-free `ApprovedValidationScope` document before any live validation approval is used.

```sh
tools/evidence-validation/validate-scope.sh
tools/evidence-validation/validate-scope.sh validation/approved-scope.v1.json
tools/evidence-validation/scope-test.sh
```

The contract requires a provider, region, account/project scope, GitHub repository, bounded resource prefix and resource count, an ISO-8601 deadline, and named notification and rollback owners. It rejects unknown fields, unsupported providers, malformed values, out-of-range resource counts, duplicate identities, and secret-shaped content. It does not authenticate or contact any cloud or source-control service.
