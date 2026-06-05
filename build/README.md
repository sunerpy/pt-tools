# pt-tools v2.0 Docker Compose Deployment

## Quick Start

This directory contains a Docker Compose template for deploying pt-tools v2.0 with CloakBrowser-Manager fallback support.

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

5. Access pt-tools at `http://localhost:8081`

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

- `sunerpy/pt-tools:v2.0.0` instead of `v2-latest`
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
- Check network: `docker compose exec pt-tools ping cloakbrowser-manager`

**Port conflicts** (8080 or 8081 already in use):

- Edit `docker-compose.yaml` ports section, e.g., `127.0.0.1:9080:8080`

### Notes

- Both services bind to **localhost only** (`127.0.0.1`) by default for security
- Persistent data is stored in named Docker volumes (`cloak_profiles`, `pttools_data`)
- Health checks ensure services are fully ready before dependent services start
- `depends_on` with `condition: service_healthy` 确保 Manager 在 pt-tools 启动前就绪（供「CloakBrowser 配置」连通性测试使用；运行时探测 fallback 集成仍在开发中）。

### Legal Compliance

**Single-image deployment**: v2.0 ships only docker-compose; single-image solutions are pending legal confirmation from CloakHQ (see [docs/legal/cloakhq-oem-inquiry.md](../docs/legal/cloakhq-oem-inquiry.md)).
