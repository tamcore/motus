# Motus Helm Chart

GPS tracking system with Traccar API compatibility for Home Assistant and Traccar Manager mobile apps. Supports H02 and WATCH GPS protocol listeners, real-time WebSocket updates, PostGIS geofencing, and multi-replica deployments with Redis pub/sub.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.10+
- A PostgreSQL 16+ database with PostGIS 3.4+ extension
- (Optional) Redis 7+ for multi-replica WebSocket broadcasting
- (Optional) cert-manager for automatic TLS certificate management
- (Optional) nginx-ingress controller for HTTP ingress

## Installation

### Development (built-in PostgreSQL)

This mode deploys a single-node PostgreSQL StatefulSet alongside Motus, suitable for testing and development.

```bash
helm install motus ./charts/motus -f charts/motus/values-dev.yaml
```

### Production (external database)

For production, disable the built-in PostgreSQL and point to your managed database cluster (e.g., CloudNativePG, Amazon RDS, Google Cloud SQL).

```bash
helm install motus ./charts/motus \
  --set postgres.enabled=false \
  --set externalDatabase.host=your-pg-host.example.com \
  --set externalDatabase.password=your-secure-password \
  --set externalDatabase.sslmode=require \
  --set ingress.hosts[0].host=motus.example.com \
  --set ingress.tls[0].hosts[0]=motus.example.com
```

#### Using a connection URI from a Secret (CloudNativePG)

If your database operator provides a connection URI in a Kubernetes Secret (common with CloudNativePG), use the `postgresUriSecret` option instead:

```bash
helm install motus ./charts/motus \
  --set postgres.enabled=false \
  --set postgresUriSecret.enabled=true \
  --set postgresUriSecret.name=motus-postgresql-app \
  --set postgresUriSecret.key=uri
```

## Database Migrations

Migrations run automatically as a Helm post-install/post-upgrade Job. The job uses a `wait-for-db` init container to ensure the database is reachable before applying migrations with goose.

No manual migration steps are required.

## Configuration Reference

### Image

| Parameter | Description | Default |
|---|---|---|
| `image.repository` | Container image registry/name | `ghcr.io/tamcore/motus` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `Always` |

### Application

| Parameter | Description | Default |
|---|---|---|
| `replicaCount` | Number of Motus pods | `2` |
| `config.server.port` | HTTP API port | `8080` |
| `config.gps.h02Port` | H02 GPS protocol port | `5013` |
| `config.gps.watchPort` | WATCH GPS protocol port | `5093` |

### Services

| Parameter | Description | Default |
|---|---|---|
| `service.type` | HTTP service type | `ClusterIP` |
| `service.port` | HTTP service port | `8080` |
| `serviceGPS.type` | GPS service type | `LoadBalancer` |
| `serviceGPS.loadBalancerIP` | Static IP for GPS LoadBalancer | `""` |
| `serviceGPS.ports.h02` | H02 listener port | `5013` |
| `serviceGPS.ports.watch` | WATCH listener port | `5093` |

### Ingress

| Parameter | Description | Default |
|---|---|---|
| `ingress.enabled` | Enable HTTP ingress | `true` |
| `ingress.className` | Ingress class name | `nginx` |
| `ingress.hosts` | List of host rules | see values.yaml |
| `ingress.tls` | TLS configuration | see values.yaml |
| `ingress.annotations` | Additional ingress annotations | `{}` |

The chart automatically configures nginx annotations for WebSocket support (3600s timeouts) and cert-manager TLS.

### Built-in PostgreSQL (development only)

| Parameter | Description | Default |
|---|---|---|
| `postgres.enabled` | Deploy a PostgreSQL StatefulSet | `false` |
| `postgres.image` | PostgreSQL image with PostGIS | `postgis/postgis:16-3.4` |
| `postgres.storage` | PVC storage size | `10Gi` |
| `postgres.storageClass` | Storage class name | `""` |
| `postgres.database` | Database name | `motus` |
| `postgres.username` | Database user | `motus` |
| `postgres.password` | Database password | `motus123` |

### External Database

| Parameter | Description | Default |
|---|---|---|
| `externalDatabase.host` | PostgreSQL host | `""` |
| `externalDatabase.port` | PostgreSQL port | `5432` |
| `externalDatabase.database` | Database name | `motus` |
| `externalDatabase.username` | Database user | `motus` |
| `externalDatabase.password` | Database password | `""` |
| `externalDatabase.sslmode` | SSL mode | `require` |

### PostgreSQL URI Secret (CloudNativePG)

| Parameter | Description | Default |
|---|---|---|
| `postgresUriSecret.enabled` | Use a Secret for the connection URI | `false` |
| `postgresUriSecret.name` | Secret name | `""` |
| `postgresUriSecret.key` | Key within the Secret | `""` |

### Redis

Redis is required for multi-replica WebSocket broadcasting. Without it, each pod only broadcasts to its own local clients.

| Parameter | Description | Default |
|---|---|---|
| `redis.enabled` | Enable Redis pub/sub | `false` |
| `redis.external.enabled` | Use an external Redis instance | `false` |
| `redis.external.url` | External Redis URL | `""` |
| `redis.builtin.enabled` | Deploy a built-in Redis StatefulSet | `false` |
| `redis.builtin.image` | Redis image | `redis:7-alpine` |
| `redis.builtin.storage` | PVC storage size | `1Gi` |
| `redis.builtin.storageClass` | Storage class name | `""` |

### WebSocket

| Parameter | Description | Default |
|---|---|---|
| `websocket.allowedOrigins` | Comma-separated list of allowed WebSocket origins | `""` |

Set this to your domain (e.g., `https://motus.example.com`) in production. An empty value allows only localhost origins.

### Metrics (Prometheus)

| Parameter | Description | Default |
|---|---|---|
| `metrics.enabled` | Enable Prometheus metrics endpoint | `false` |
| `metrics.port` | Metrics server port | `9090` |
| `metrics.serviceMonitor.enabled` | Create a Prometheus ServiceMonitor | `false` |
| `metrics.serviceMonitor.interval` | Scrape interval | `30s` |
| `metrics.serviceMonitor.labels` | Additional labels for ServiceMonitor | `{}` |

### Demo Mode

| Parameter | Description | Default |
|---|---|---|
| `demo.enabled` | Enable demo mode with simulated GPS tracks | `false` |
| `demo.gpxDir` | Path to GPX route files in the container | `/data/demo` |
| `demo.resetTime` | Time of day (HH:MM) to reset demo data | `00:00` |
| `demo.deviceIMEIs` | Comma-separated demo device identifiers | `""` |
| `demo.speedMultiplier` | Simulation speed (1.0 = real time) | `10.0` |
| `demo.interpolationInterval` | Max distance in meters between route points | `100.0` |

When enabled, a separate demo pod runs the GPS simulator targeting the main app's H02 service.

### Resources

| Parameter | Description | Default |
|---|---|---|
| `resources.limits.cpu` | CPU limit | `1000m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |

### High Availability

| Parameter | Description | Default |
|---|---|---|
| `podDisruptionBudget.enabled` | Enable PDB | `true` |
| `podDisruptionBudget.minAvailable` | Minimum available pods during disruptions | `1` |

## External Service Requirements

### PostgreSQL with PostGIS

Motus requires PostgreSQL 16+ with the PostGIS extension for geofencing. The database must have the `postgis` extension available. Migrations create it automatically (`CREATE EXTENSION IF NOT EXISTS postgis`).

**Recommended managed options:**
- CloudNativePG (Kubernetes operator)
- Amazon RDS for PostgreSQL (with PostGIS)
- Google Cloud SQL for PostgreSQL
- Azure Database for PostgreSQL Flexible Server

### Redis (optional)

Required only when running multiple replicas (`replicaCount > 1`). Redis provides cross-pod pub/sub so WebSocket broadcasts reach all connected clients regardless of which pod they are connected to.

Any Redis 7+ instance works. The chart can deploy a built-in single-node Redis, or you can use an external instance.

## Storage Class Configuration

The built-in PostgreSQL and Redis StatefulSets require a storage class that supports `ReadWriteOnce` access mode.

### Common Cloud Providers

| Provider | Storage Class | Notes |
|---|---|---|
| AWS EKS | `gp3` or `gp2` | EBS volumes |
| Google GKE | `standard` or `premium-rwo` | Persistent Disk |
| Azure AKS | `managed-premium` or `default` | Azure Disk |
| Hetzner | `hcloud-volumes` | Hetzner Cloud Volumes |
| k3s (local) | `local-path` | Local storage (not HA) |

Set the storage class in values:

```yaml
postgres:
  storageClass: gp3    # AWS example
  storage: 20Gi

redis:
  builtin:
    storageClass: gp3
    storage: 1Gi
```

## Upgrade Instructions

### Standard Upgrade

```bash
helm upgrade motus ./charts/motus -f your-values.yaml
```

Database migrations run automatically via a post-upgrade hook Job. The rolling update strategy ensures zero downtime (`maxUnavailable: 0, maxSurge: 1`).

### Rollback

```bash
helm rollback motus <revision>
```

Note: Database migrations are forward-only. Rolling back the chart does not revert schema changes. The application is designed to be backward-compatible with newer schemas.

## Architecture

```
                    Internet
                       |
              +--------+--------+
              |                 |
         nginx-ingress    LoadBalancer
         (HTTP/WS)        (TCP 5013/5093)
              |                 |
         +----+----+      +----+----+
         |  Motus  |      |  Motus  |
         | Pod #1  |      | Pod #2  |
         +----+----+      +----+----+
              |    \      /    |
              |     Redis      |
              |   (pub/sub)    |
              +-------+--------+
                      |
                 PostgreSQL
                 (PostGIS)
```

- **HTTP/WebSocket traffic** routes through the ingress controller to Motus pods
- **GPS device traffic** (H02/WATCH protocols) routes through the LoadBalancer directly to pods
- **Redis** enables real-time WebSocket broadcasts across pods
- **PostgreSQL** stores all position data, device state, geofences, and user configuration
