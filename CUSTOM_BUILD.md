# Custom build & Upstream Tracking

This fork tracks `Mag1cFall/AIStudio2API` and maintains atomic private patches in `patches/`.
Patches are applied during CI verification and Docker build. The upstream source tree in Git remains unpolluted.

## Atomic Patch Registry

See [`patches/PATCHES.md`](patches/PATCHES.md) for full descriptions of all private customizations:
- `patches/01-quota-cooldown.patch` — Stepped quota backoff (2m / 15m)
- `patches/02-reset-cooldowns.patch` — Reset cooldowns upon account verification
- `patches/03-schema-lenient.patch` — Lenient JSON Schema protobuf handling
- `patches/04-webui-auth.patch` — Password / API Key protected Web UI & loopback bypass
- `patches/05-camoufox-run.patch` — Camoufox Run button selector fallback

## Image

```text
ghcr.io/bundle5936/aistudio-omni:latest
```

The image workflow builds `linux/amd64` and `linux/arm64` and publishes both `latest` and immutable `sha-*` tags.

## Automated Sync

`.github/workflows/sync-upstream.yml` periodically checks `Mag1cFall/AIStudio2API:main`.
- If clean and patches apply, tests and pushes to `main` -> triggers image build.
- If upstream implements a feature or conflicts, CI halts, files an issue with structured instructions, and alerts for review.
