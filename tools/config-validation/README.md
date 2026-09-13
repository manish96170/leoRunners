# Configuration validation

`validate.sh` performs an offline contract check for the controller's
production configuration boundary. It verifies that secret values remain
out-of-band, Kubernetes references use the declared Secret keys, diagnostics
expose presence only, and JIT, AI, and extensions remain disabled by default.

The validator never reads process credentials, contacts a cloud provider, or
applies Kubernetes resources. Run `./test.sh` from this directory for the
focused fixture and deployment checks.
