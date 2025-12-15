# Quick Fix for Port 9091 Access

## Problem
Port 9091 is not exposed from `milvus-builder-1` container, so the dashboard can't connect.

## Solution Options

### Option 1: Restart Container with Port Exposed (Recommended)

The `docker-compose.yml` has been updated. Restart the container:

```bash
# Stop and restart the builder container
docker-compose restart builder

# OR if that doesn't work, recreate it:
docker-compose stop builder
docker-compose up -d builder
```

Then verify port is exposed:
```bash
docker ps --filter "name=milvus-builder-1" --format "{{.Ports}}"
# Should show: 0.0.0.0:19530->19530/tcp, 0.0.0.0:9091->9091/tcp
```

### Option 2: Use Docker Port Forward (Temporary)

If you can't restart, use port forwarding:

```bash
# In one terminal, keep this running:
docker exec -it milvus-builder-1 bash -c "while true; do socat TCP-LISTEN:9091,fork,reuseaddr TCP:localhost:9091 2>/dev/null || sleep 1; done"

# OR use SSH tunnel approach via docker exec
docker exec -it milvus-builder-1 bash -c "apt-get update && apt-get install -y socat && socat TCP-LISTEN:9091,fork TCP:localhost:9091"
```

### Option 3: Access via Container IP

Find the container's IP and access directly:

```bash
# Get container IP
CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' milvus-builder-1)
echo "Container IP: $CONTAINER_IP"

# Update dashboard host to: http://$CONTAINER_IP:9091
```

### Option 4: Use Docker Network Proxy

Create a simple proxy container:

```bash
docker run -d --name milvus-proxy \
  --network milvus_dev \
  -p 9091:9091 \
  alpine/socat \
  tcp-listen:9091,fork,reuseaddr tcp-connect:milvus-builder-1:9091
```

## Verify Fix

After applying any solution, test:

```bash
# From host
curl http://localhost:9091/healthz
# Should return: OK

# Test CBO endpoint
curl http://localhost:9091/api/v1/_cbo/metrics
```

Then refresh the dashboard - it should connect successfully!
