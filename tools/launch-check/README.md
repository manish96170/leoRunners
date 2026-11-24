# launch-check

A read-only check of one launched runner instance against the runner contract.
Use it as evidence for the first real launch, and again after any change to the
launch template, AMI or controller.

```sh
tools/launch-check/check.sh --instance-id i-0123456789abcdef0 \
  --region us-east-1 --request-id <your request id> \
  --profile-arn <expected instance profile ARN>
```

It calls only `ec2 describe-instances`, `describe-instance-attribute` and
`describe-volumes`. It changes nothing and needs no more than read access to
those. It prints `PASS` or `FAIL` per check and exits non-zero if any fails.

| Check | Why |
|---|---|
| instance is pending or running | the launch happened and was not reaped |
| no public IP, one network interface deleted with the instance | the template owns the interface |
| IMDSv2 required, hop limit 1, instance metadata tags off | no metadata theft from workflows or containers |
| shutdown behaviour is `terminate` | a finished runner leaves nothing behind |
| `leo-runners:managed-by`, `leo-runners:runner-id`, `leo-runners:expires-at` on the instance | the reaper, its backstop and IAM conditions depend on them |
| `--request-id` (as `RequestId`) and any `--tag KEY=VALUE` on the instance **and every volume** | request-scoped cleanup |
| instance profile present, and equal to `--profile-arn` if given | the dedicated runner identity |
| every volume encrypted, gp3, deleted with the instance, ownership-tagged | no leaked or unencrypted storage |

It cannot check that IMDSv1 is refused at runtime. It prints the command to run
on the instance (for example through Session Manager); the answer must be `401`.

`test.sh` runs the checker against a stub `aws` and fixtures, with one good
launch and sixteen deliberately broken ones, so the checker itself is tested
without touching any cloud.
