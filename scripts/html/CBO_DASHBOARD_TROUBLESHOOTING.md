# CBO Dashboard Troubleshooting Guide

## Connection Refused Errors

If you see `ERR_CONNECTION_REFUSED` when trying to access the dashboard, it usually means:

### Issue: Port 9091 Not Exposed

When Milvus is running inside a Docker container (like `milvus-builder-1`), port 9091 may not be exposed to the host.

#### Solution 1: Expose Port 9091 in docker-compose.yml

Edit `docker-compose.yml` and ensure the builder service exposes port 9091:

```yaml
services:
  builder:
    ports:
      - "19530:19530"
      - "9091:9091"  # Add this line
```

Then restart the container:
```bash
docker-compose down builder
docker-compose up -d builder
```

#### Solution 2: Use Docker Port Forwarding

If you can't modify docker-compose.yml, use port forwarding:

```bash
# Forward port 9091 from container to host
docker port milvus-builder-1 9091
# Or use socat (if available)
socat TCP-LISTEN:9091,fork TCP:milvus-builder-1:9091
```

#### Solution 3: Access via Container Network

If containers are on the same Docker network, you can access via container name:

1. Find the container's IP:
   ```bash
   docker inspect milvus-builder-1 | grep IPAddress
   ```

2. Update dashboard host to use container IP or use Docker's internal networking

#### Solution 4: Use SSH Tunnel (for remote access)

If accessing remotely:
```bash
ssh -L 9091:localhost:9091 user@remote-host
```

### Issue: Milvus Not Running

Check if Milvus is actually running:

```bash
# From inside the container
docker exec milvus-builder-1 curl http://localhost:9091/healthz

# Should return: OK
```

If not running, start Milvus:
```bash
docker exec milvus-builder-1 bash -c "cd /go/src/github.com/milvus-io/milvus && ./bin/milvus run standalone"
```

### Issue: CORS Errors

If you see CORS errors when opening the HTML file directly (`file://`), serve it via HTTP server:

```bash
cd scripts/html
python3 -m http.server 8000
# Then open: http://localhost:8000/cbo_dashboard.html
```

### Issue: Wrong Host Configuration

Make sure the dashboard host is set correctly:

- **Local Milvus**: `http://localhost:9091`
- **Container Milvus**: `http://localhost:9091` (if port is exposed) or container IP
- **Remote Milvus**: `http://<remote-ip>:9091`

## Chart.js Loading Issues

If you see tracking prevention warnings for Chart.js:

1. The dashboard will still work (tables will display)
2. Charts will be disabled
3. To fix: Serve the HTML via HTTP server instead of opening directly

## Quick Diagnostic Commands

```bash
# Check if port 9091 is exposed
docker port milvus-builder-1 9091

# Test connection from host
curl http://localhost:9091/healthz

# Test connection from container
docker exec milvus-builder-1 curl http://localhost:9091/healthz

# Check if Milvus is running
docker exec milvus-builder-1 ps aux | grep milvus

# Check port bindings
docker ps --filter "name=milvus-builder-1" --format "{{.Ports}}"
```
