# pt-tools + CloakBrowser-Manager Docker Compose example

## Quick Start

This directory contains a Docker Compose template that runs pt-tools next to CloakBrowser-Manager, which pt-tools can use as a fallback for site login probes (see [Site login monitoring](../docs/en/guide/site-login-monitoring.md)).

### Prerequisites

- Docker & Docker Compose
- `.env` file with `CLOAK_AUTH_TOKEN` configured

### Deployment Steps

1. Copy the example environment file:

   ```bash
   cp docker-compose.example.env .env
   ```

2. Edit `.env` and set a secure `CLOAK_AUTH_TOKEN`:

   ```bash
   # Generate a new token:
   openssl rand -hex 32
   ```

3. Start the services:

   ```bash
   cd build
   docker compose up -d
   ```

4. Verify both services are healthy:

   ```bash
   docker compose ps
   ```

   Look for `healthy` status on both `pt-tools-cloak` and `pt-tools` containers.

5. Access pt-tools at `http://localhost:8081` (default login `admin` / `adminadmin`; change it after the first login).

6. Create a profile in the CloakBrowser-Manager UI at `http://localhost:8080`.

7. In pt-tools, open 系统 → CloakBrowser and fill in:

   - CloakBrowser-Manager 端点: `http://cloakbrowser-manager:8080` (the service name inside the compose network, not `localhost`)
   - Auth Token: the `CLOAK_AUTH_TOKEN` from `.env`
   - Profile ID: the profile created in step 6

   测试连接 checks the endpoint and token only. The fallback is used only when all three are set. pt-tools does not read these settings from environment variables.

   Profile ID and the login-probe fallback come with v1.0.0 (and the `v1.0.0-rc.N` previews); earlier versions only have the endpoint and token, which 测试连接 uses. Until v1.0.0 is out, `latest` is an earlier version: pin a `v1.0.0-rc.N` tag to use the fallback.

### Common Operations

**View logs**:

```bash
docker compose logs -f pt-tools
docker compose logs -f cloakbrowser-manager
```

**Stop services** (preserves volumes):

```bash
docker compose down
```

**Stop services and delete all data** (destructive):

```bash
docker compose down -v
```

### Updating Images

**Pull latest images**:

```bash
docker compose pull
docker compose up -d
```

**Pin to specific versions**:
Edit the `image:` field in `docker-compose.yaml`, e.g.:

- `sunerpy/pt-tools:<version>` (for example `sunerpy/pt-tools:v1.0.0`) instead of `latest`
- `cloakhq/cloakbrowser-manager:0.0.4@sha256:<digest>` for SHA-pinned Manager

### Manager Image SHA Digest Pinning (R21)

For production, pin the CloakBrowser-Manager image to a specific digest to prevent unexpected updates:

1. **Discover the current digest**:

   ```bash
   docker pull cloakhq/cloakbrowser-manager:0.0.4
   docker inspect cloakhq/cloakbrowser-manager:0.0.4 | jq '.[0].RepoDigests[0]'
   ```

2. **Update `docker-compose.yaml`**:

   ```yaml
   image: cloakhq/cloakbrowser-manager:0.0.4@sha256:<digest>
   ```

3. **Re-deploy**:
   ```bash
   docker compose up -d
   ```

### Troubleshooting

**Services not starting**:

- Check `.env` file exists and `CLOAK_AUTH_TOKEN` is set
- Verify Docker daemon is running: `docker ps`

**pt-tools cannot reach CloakBrowser-Manager**:

- Verify Manager is healthy: `docker compose logs cloakbrowser-manager | tail -20`
- Check that the endpoint in 系统 → CloakBrowser is `http://cloakbrowser-manager:8080`: inside the pt-tools container, `localhost` is pt-tools itself
- Check network: `docker compose exec pt-tools wget -qO- http://cloakbrowser-manager:8080/api/status`

**Port conflicts** (8080 or 8081 already in use):

- Edit the host side of the `ports` entries in `docker-compose.yaml`, e.g. `127.0.0.1:9080:8080`

### Notes

- Both services bind to **localhost only** (`127.0.0.1`) by default for security
- Persistent data is stored in named Docker volumes (`cloak_profiles`, `pttools_data`); pt-tools keeps its database and `secret.key` in `/app/.pt-tools`, back up `secret.key` together with the database
- Health checks ensure services are fully ready before dependent services start; pt-tools answers `GET /api/ping` without login. Its check runs `wget -Y off` because busybox wget ignores `no_proxy`, and Docker's client proxy settings (`~/.docker/config.json`) put `HTTP_PROXY` into the container

### Legal Compliance

**Separate containers**: CloakBrowser-Manager runs as its own container and is not bundled into the pt-tools image; a single-image setup is pending legal confirmation from CloakHQ.
