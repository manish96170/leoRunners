# Image Publication Contract

The Fargate service runs one container image, referenced by an **immutable
digest**. The module refuses mutable tags: `image_digest` must match
`sha256:<64 lowercase hex>`, and the ECR repository it creates has immutable
tags, scan on push, and a lifecycle policy.

There is currently no official public LeoRunners image. Each deployment
builds from a reviewed revision and publishes to a registry it controls.

## Contract

- Build from the repository root with the checked-in `Dockerfile` at an exact commit (or release tag). Record the commit.
- Build for the architecture the task runs: `X86_64` (default) maps to `linux/amd64`, `ARM64` to `linux/arm64`. Set `cpu_architecture` to match.
- The image runs as UID/GID `65532`, listens on `LISTEN_ADDR` (default `:8080`), serves `GET /healthz`, and ships no credentials. Configuration comes from environment variables and Secrets Manager references at runtime.
- Tag with a version label for humans (tags are immutable in the created repository) and deploy by digest. Never deploy `latest`.
- Review the ECR scan findings before the digest is approved.
- Record, with the change request: source commit, build date, builder, digest, and scan result.

## Publish to the created repository

```sh
# Outputs from phase 1
REPO_URL=$(terraform output -raw ecr_repository_url)
REGION=us-east-1
TAG=v0.1.0

aws ecr get-login-password --region "$REGION" \
  | docker login --username AWS --password-stdin "${REPO_URL%%/*}"

docker buildx build --platform linux/amd64 -t "$REPO_URL:$TAG" --push .

aws ecr describe-images --region "$REGION" \
  --repository-name "${REPO_URL#*/}" --image-ids imageTag="$TAG" \
  --query 'imageDetails[0].imageDigest' --output text
```

Pass the printed digest as `image_digest`. The service uses
`<repository_url>@<digest>`; the tag is not part of the reference.

## Using an existing registry

Set `create_ecr_repository = false` and `image_repository` to the repository
URI (without tag or digest). Grant the ECS execution role pull access through
that registry's own policy.

## Rollback

Point `image_digest` at a previously approved digest. The lifecycle policy
keeps the most recent `ecr_max_image_count` images (default 50), so keep that
above the number of digests you may roll back to.

## Notes

- The `STATE_PATH` default baked into the image is ignored when `DYNAMODB_TABLE_NAME` is set, which the module always does. The container runs with a read-only root filesystem and all Linux capabilities dropped.
- A signed or attested image is better than an unsigned one. The module does not enforce signing; add it in your registry or admission process.
