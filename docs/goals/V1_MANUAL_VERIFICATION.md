# GamePanel Lite V1 Manual Verification

Use this checklist for the final manual verification that cannot be proven from automated tests alone.

## Terraria Client Join

Prerequisites:
- Docker daemon is running.
- Go API is running with a working `GAMEPANEL_DOCKER_HOST`.
- Web app is running and points to the Go API.
- Terraria desktop client is installed on the same machine or another machine that can reach the host IP.

### Vanilla Join

1. Create a Vanilla Terraria server from the web UI.
2. Start the server.
3. Open the server detail page and wait for logs to show `Listening on port <port>` and `Server started`.
4. In the Terraria desktop client, choose Multiplayer, then Join via IP.
5. Use the host IP shown in the panel. For same-machine verification, use `127.0.0.1`.
6. Enter the configured port and password if one was set.
7. Verify the client joins the world.
8. Verify the server detail log stream shows a client connection or join-related log line.
9. Stop and delete the temporary server from the panel.

### tModLoader Join

1. Create a tModLoader Terraria server from the web UI.
2. Start the server.
3. Open the server detail page and wait for logs to show `Listening on port <port>` and `Server started`.
4. Open a matching tModLoader desktop client.
5. Join via IP using the host IP and configured port.
6. Enter the password if one was set.
7. Verify the client joins the world.
8. Verify the server detail log stream shows a client connection or join-related log line.
9. Stop and delete the temporary server from the panel.

## Pass Criteria

V1 manual join verification is complete when both Vanilla and tModLoader clients can join servers created and started from GamePanel Lite, and the temporary containers are cleaned up afterward.
