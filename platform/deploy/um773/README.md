# um773 deployment

This deployment runs the current single-host preview implementation behind one HTTP entry point. It intentionally does not configure PostgreSQL, NATS, or Docker workload mutation until the production message relay and deployment seeding path exist.

From the `platform` directory on um773:

```sh
GAMEPANEL_RELEASE_TAG=<git-sha> GAMEPANEL_PUBLIC_PORT=3005 docker compose -f deploy/um773/compose.yaml up -d --build
```

Health checks:

```sh
curl --fail http://127.0.0.1:3005/health
curl --fail http://127.0.0.1:3005/
```

Rollback is performed by stopping the `gamepanel-platform` Compose project and restarting `/opt/game-panel-lite/docker-compose.um773.yaml`.
