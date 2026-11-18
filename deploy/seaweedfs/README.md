# SeaweedFS Object Storage

GamePanel Lite only depends on the standard AWS S3 V4 API (`GET`/`PUT`/`HEAD`/`DELETE`/`ListObjectsV2`).
SeaweedFS (Apache-2.0) is the default backend; any S3-compatible store (Ceph RGW, OSS, COS, R2) can be swapped in
by changing `GP_S3_ENDPOINT` / `GP_S3_PUBLIC_ENDPOINT`.

## Development (single node, `docker-compose.yaml`)

| Service | Port | Purpose |
| --- | --- | --- |
| `seaweedfs` | `8333` | S3 API (`GP_S3_ENDPOINT=http://seaweedfs:8333`) |
| `seaweedfs` | `127.0.0.1:9333` | Master status |
| `seaweedfs-admin` | `23646` | Built-in admin console (buckets / volumes / cluster) |

Create the bucket once:

```bash
docker exec gamepanel-seaweedfs sh -c 'echo "s3.bucket.create -name gamepanel-backups" | weed shell -master=seaweedfs:9333'
```

## Production (dedicated storage nodes, independent from the control plane)

```
control plane (apiserver/controller) ──internal S3──┐
                                                    ▼
                                   VIP / HAProxy :8333 (S3)
                                   ├── storage-1: master + volume + filer + s3
                                   ├── storage-2: master + volume + filer + s3
                                   └── storage-3: master + volume + filer + s3
compute nodes (node-agent, optional Dragonfly) ──presigned URL──▲
```

Recommendations:

1. **3 storage nodes**: masters form a Raft quorum (`-master.peers=s1:9333,s2:9333,s3:9333`).
2. **Replication**: `-master.defaultReplication=001` (one extra copy on another server in the same rack);
   use `010` once nodes span racks.
3. **Filer metadata**: a dedicated PostgreSQL for the storage tier (`filer.toml` `[postgres2]`),
   never shared with the control-plane database.
4. **Two endpoints**: `GP_S3_ENDPOINT` (internal, control plane) and `GP_S3_PUBLIC_ENDPOINT`
   (reachable by compute nodes for presigned package downloads, keeping bulk traffic off the control plane).
5. **Large fleets**: run Dragonfly seed peers next to the storage nodes and set
   `GP_AGENT_DOWNLOAD_PROXY` on node-agents for P2P package distribution.
6. **Admin console**: run `weed admin` behind SSO / internal network and link it from the platform console.
7. **Bucket hygiene**: the controller-manager runs a daily orphan-object reconciliation (Mark & Sweep) against
   database references; incomplete multipart uploads are left to the storage backend's own cleanup.

Migrating to another S3 backend later: `rclone sync old:gamepanel-backups new:gamepanel-backups`,
then switch the two endpoints.
