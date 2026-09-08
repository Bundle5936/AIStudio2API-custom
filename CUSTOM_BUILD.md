# Custom build

This fork tracks `Mag1cFall/AIStudio2API` and applies `custom.patch` only during
the Docker build. The upstream source tree is not directly modified.

## Image

```text
ghcr.io/bundle5936/aistudio-omni:latest
```

The image workflow builds `linux/amd64` and `linux/arm64` and publishes both
`latest` and immutable `sha-*` tags.

## Local deployment

Copy `docker-compose.example.yml` to `docker-compose.yml`, create a local `.env`,
and keep `auth/` and `runtime/` outside Git. Never commit API keys or browser
authentication state.
