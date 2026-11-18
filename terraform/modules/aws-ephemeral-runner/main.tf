# This module deliberately creates no resources. It validates the inputs for
# the future ephemeral runner layer and publishes the contract that the
# launch template, controller policy review, and cleanup tooling must honor.

locals {
  fixed_tags = {
    Platform  = "ci-runner"
    ManagedBy = "leo-runners"
  }

  ownership_tags = merge(
    var.tags,
    var.request_id == null ? {} : { RequestId = var.request_id },
    var.cleanup_deadline == null ? {} : { CleanupDeadline = var.cleanup_deadline },
    local.fixed_tags,
  )

  account_ids = distinct([
    split(":", var.runner_role_arn)[4],
    split(":", var.runner_instance_profile_arn)[4],
    split(":", var.controller_role_arn)[4],
  ])
}
