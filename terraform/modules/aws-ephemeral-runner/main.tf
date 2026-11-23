# This module deliberately creates no resources. It validates the inputs for
# the future ephemeral runner layer and publishes the contract that the
# launch template, controller policy review, and cleanup tooling must honor.

locals {
  # The ownership tag the controller's AWS provider stamps on every instance it
  # launches (controller/internal/providers/aws) and that its reaper and the
  # reviewed IAM policy select on. The provider also adds per-job tags under the
  # same "leo-runners:" prefix, which callers therefore may not set.
  ownership_tag_key   = "leo-runners:managed-by"
  ownership_tag_value = "leo-runners"

  fixed_tags = {
    (local.ownership_tag_key) = local.ownership_tag_value
  }

  ownership_tags = merge(
    var.tags,
    var.request_id == null ? {} : { RequestId = var.request_id },
    var.cleanup_deadline == null ? {} : { CleanupDeadline = var.cleanup_deadline },
    local.fixed_tags,
  )

  account_ids = distinct(concat(
    [
      split(":", var.runner_role_arn)[4],
      split(":", var.runner_instance_profile_arn)[4],
    ],
    var.controller_role_arn == null ? [] : [split(":", var.controller_role_arn)[4]],
  ))
}
