# Event retention validation

`validate.sh` checks a JSON `EventArchiveRetention` policy without opening or
modifying the configured archive. It requires an absolute JSONL path, owner
read/write mode `0600`, append-on-restart behavior, bounded line/size/file/age
limits, and rotation files whenever a rotation trigger is enabled. The policy
contains no secret values; secret-shaped content is rejected.

Run `./test.sh` for the passing and unsafe fixtures.
