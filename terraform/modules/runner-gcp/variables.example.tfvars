# Documentation-only example values for a future runner-gcp module.
# This file creates no resources and contains no credentials.

project_id = "replace-with-disposable-gcp-project"
region     = "us-central1"

# Keep the zone list explicit and ordered for deterministic capacity fallback.
approved_zones = [
  "us-central1-a",
  "us-central1-b",
]

# Use a global or regional template created and reviewed outside this module.
instance_template = "projects/replace-with-project/global/instanceTemplates/runner-template-v1"
machine_type      = "e2-standard-2"
image_revision    = "runner-image-v1"

network    = "projects/replace-with-project/global/networks/runner-vpc"
subnetwork = "projects/replace-with-project/regions/us-central1/subnetworks/runner-private"

# The controller and VM identities must be distinct service accounts.
controller_service_account = "leo-runners-controller@replace-with-project.iam.gserviceaccount.com"
runner_service_account     = "leo-runners-vm@replace-with-project.iam.gserviceaccount.com"

# Default profile: private VM, controlled HTTPS egress, and no inbound SSH.
assign_public_ip             = false
enable_private_google_access = true
egress_mode                  = "cloud-nat"
firewall_target_tag          = "leo-runner-private"

boot_disk_type                = "pd-balanced"
boot_disk_size_gb             = 40
boot_disk_kms_key             = ""
delete_boot_disk_on_vm_delete = true

bootstrap_reference      = "gs://replace-with-approved-artifact/runner-bootstrap-v1.sh"
bootstrap_secret_channel = "one-time-protected-input"

labels = {
  platform   = "ci-runner"
  managed-by = "leo-runners"
  provider   = "gcp"
  profile    = "base-linux-x64-v1"
}

runner_ttl_seconds       = 3600
reaper_scan_interval     = "5m"
reaper_operation_timeout = "2m"
max_reaper_records       = 500

# ADC is resolved by the caller/environment. Never put a credential path or
# service-account key material in this file.
credentials_source = "application-default-credentials"

# Leave deployment disabled until a complete module, plan review, and an
# explicit approval exist.
enable_deployment = false
