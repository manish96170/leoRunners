# Phase 8 Plan

Phase 8 adds a GCP Compute Engine provider behind the existing runner-provider contract. It creates one zonal VM from a configured instance template, waits for the insert operation, polls instance status, applies deterministic ownership labels, and deletes the VM idempotently.

## Configuration

Set `GCP_INSTANCE_TEMPLATE`, `GCP_PROJECT`, and `GCP_ZONE` to select GCP mode in the controller. Authentication uses Application Default Credentials. The provider does not create templates, service accounts, networks, or firewall rules automatically.

## Real validation

1. Enable the Compute Engine API and configure ADC for the target project.
2. Create an instance template with the approved immutable image, runner service account, network, encrypted boot disk, and private egress path.
3. Grant the controller identity only the required instance create/get/delete permissions and service-account use.
4. Run the provider against a disposable zone and verify labels, operation completion, status polling, and deletion.
5. Connect the existing GitHub JIT/bootstrap path only after the standalone VM lifecycle succeeds.

The mocked provider tests make no GCP calls. The implementation follows the documented Compute Engine instance-template creation flow and zonal instance deletion flow.
