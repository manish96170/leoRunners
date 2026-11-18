# Workload Intake

This offline tool derives a redacted workload manifest and evidence report from
a local, clean checkout at one pinned commit. It reads only a bounded allowlist
of workflow, build, runtime, dependency, container, and infrastructure files.

```sh
go run . --repository /path/to/checkout --commit <40-char-head-commit> \
  --output /tmp/workload-intake.json
```

The command fails closed when the checkout is dirty, `HEAD` does not exactly
match the supplied full commit, the file or byte bound is exceeded, or a
secret-shaped value is found. File contents are never copied into the report;
the report contains only paths, coarse locators, capability IDs, counts, and
redaction attestations. Output files are written atomically with mode `0600`.

No workflow, shell, build, test, package-manager, Docker, Terraform, or cloud
command is executed. Git is used only for identity and cleanliness checks.

The report is evidence of repository inspection, not proof that the workload
passes. Run the separate workload-validation tool only after a human reviews
the derived manifest and explicitly chooses a command.
