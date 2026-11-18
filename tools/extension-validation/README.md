# Extension runtime validation

`validate.sh` performs offline validation of the versioned extension runtime
policy. It requires only Ruby's standard YAML and JSON libraries.

The policy is fail-closed for unknown consumer IDs, wildcard or out-of-scope
tenant grants, duplicate identities, unbounded queues or timeouts, unsafe
capabilities, missing metadata-only redaction, secret-shaped values, and
command or raw-data fields. Valid policies grant observation and advisory
emission only; they cannot control lifecycle or credentials.

Run the focused fixture tests from the repository root:

    ./tools/extension-validation/test.sh
