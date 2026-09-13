# IAM Policy Validation

This is an offline, review-only contract checker for the externally generated
controller runtime policy. It never calls IAM, uploads a policy, applies
Terraform, or changes AWS state.

The contract requires explicit action and resource boundaries. It rejects
unrestricted actions/resources, checks `iam:PassRole` for an exact
`iam:PassedToService` condition, and verifies DynamoDB table and index ARN
boundaries. The included simulations exercise expected allows and denies,
including an out-of-scope runner role and table.

```sh
./test.sh
./validate.sh --policy fixtures/policy-safe.json --contract fixtures/contract-safe.json
```

Use the generated policy output from the approved `iam-policy-autopilot`
workflow as input. Review the output and retain it as an audit artifact; this
tool is deliberately not an AWS IAM policy simulator.
