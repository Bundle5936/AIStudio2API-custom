# Downstream customization

This fork directly maintains its private behavior changes in the Go source tree. The Docker build compiles the checked-in source as-is; it does not apply a separate patch at build time.

## Current customizations

The downstream changes are currently concentrated in:

- `internal/aistudio/accounts.go` and `internal/app/admin.go` — clear account cooldown state after successful verification.
- `internal/aistudio/quota.go` — use short quota backoff intervals instead of a full-day cooldown.
- `internal/aistudio/schema.go` — tolerate unsupported schema fields and incomplete array schemas.
- `internal/api/middleware.go` and `internal/app/app.go` — protect the management UI with API-key login.
- `internal/camoufoxnative/worker.go` — tolerate changes to the AI Studio Run button.

These are ordinary source changes, not Docker-only patches. Keep related changes in clear commits so upstream synchronization can identify the downstream files that need review.

## Automated upstream policy

`.github/workflows/sync-upstream.yml` compares the files changed by the new upstream revision with the files changed locally since the last common upstream revision.

- If the file sets do not overlap, the workflow merges upstream, runs validation, and updates `main` automatically.
- If they overlap, the workflow stops before changing `main` and creates a review issue containing the upstream commit and affected files.
- The reviewer decides whether to keep, adapt, or remove the downstream source change. A successful validation then triggers the normal image publication workflow.

## Image

```text
ghcr.io/bundle5936/aistudio-omni:latest
```

The image workflow builds `linux/amd64` and `linux/arm64` and publishes both `latest` and immutable `sha-*` tags.
