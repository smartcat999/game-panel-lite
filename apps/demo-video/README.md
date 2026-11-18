# GamePanel Lite demo video

The product preview is built entirely from maintainable Remotion scenes. It does
not contain production screenshots, server addresses, or credentials.

## Commands

Preview the composition:

```console
pnpm --dir apps/demo-video dev
```

Render either localized 1080p MP4, poster, and README GIF from the repository root:

```console
pnpm --dir apps/demo-video render:zh
pnpm --dir apps/demo-video poster:zh
pnpm --dir apps/demo-video gif:zh

pnpm --dir apps/demo-video render:en
pnpm --dir apps/demo-video poster:en
pnpm --dir apps/demo-video gif:en
```

Validate the source:

```console
pnpm --dir apps/demo-video lint
```
