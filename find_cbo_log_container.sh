#!/bin/bash
# Script to locate CBO log file inside Milvus Docker container

CONTAINER_NAME="${1:-milvus-builder-1}"

echo "Searching for CBO log file (cbo.log) inside Docker container: $CONTAINER_NAME"
echo ""

# Check if container exists
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "⚠ Container '$CONTAINER_NAME' is not running."
    echo "Available containers:"
    docker ps --format '  {{.Names}}'
    exit 1
fi

echo "=== Checking Milvus process ==="
MILVUS_RUNNING=$(docker exec "$CONTAINER_NAME" pgrep -f "[m]ilvus" | wc -l)
if [ "$MILVUS_RUNNING" -gt 0 ]; then
    echo "✓ Milvus is running ($MILVUS_RUNNING process(es))"
    MILVUS_PID=$(docker exec "$CONTAINER_NAME" pgrep -f "[m]ilvus" | head -1)
    WORK_DIR=$(docker exec "$CONTAINER_NAME" readlink -f /proc/$MILVUS_PID/cwd 2>/dev/null || echo "/go/src/github.com/milvus-io/milvus")
    echo "  Process ID: $MILVUS_PID"
    echo "  Working directory: $WORK_DIR"
    echo ""
else
    echo "⚠ Milvus process not found"
    WORK_DIR="/go/src/github.com/milvus-io/milvus"
fi

echo "=== Searching for cbo.log ==="
echo ""

# Search in common locations
LOCATIONS=(
    "$WORK_DIR/logs/cbo.log"
    "/tmp/milvus/logs/cbo.log"
    "/go/src/github.com/milvus-io/milvus/logs/cbo.log"
    "/var/lib/milvus/logs/cbo.log"
    "/tmp/cbo.log"
)

FOUND=false
for loc in "${LOCATIONS[@]}"; do
    if docker exec "$CONTAINER_NAME" test -f "$loc" 2>/dev/null; then
        echo "✓ Found CBO log file at: $loc"
        SIZE=$(docker exec "$CONTAINER_NAME" ls -lh "$loc" 2>/dev/null | awk '{print $5}')
        MODIFIED=$(docker exec "$CONTAINER_NAME" stat -c "%y" "$loc" 2>/dev/null || docker exec "$CONTAINER_NAME" stat -f "%Sm" "$loc" 2>/dev/null)
        echo "  Size: $SIZE"
        echo "  Last modified: $MODIFIED"
        echo ""
        echo "To view the log file:"
        echo "  docker exec $CONTAINER_NAME cat $loc"
        echo "  docker exec $CONTAINER_NAME tail -f $loc"
        FOUND=true
        break
    fi
done

if [ "$FOUND" = false ]; then
    echo "⚠ CBO log file not found in common locations."
    echo ""
    echo "Searching more broadly inside container..."
    echo ""
    
    # Search in common directories
    echo "Searching $WORK_DIR/logs..."
    docker exec "$CONTAINER_NAME" find "$WORK_DIR/logs" -name "cbo.log" 2>/dev/null | head -5
    
    echo ""
    echo "Searching /tmp..."
    docker exec "$CONTAINER_NAME" find /tmp -name "cbo.log" 2>/dev/null | head -5
    
    echo ""
    echo "Searching /go/src/github.com/milvus-io/milvus..."
    docker exec "$CONTAINER_NAME" find /go/src/github.com/milvus-io/milvus -name "cbo.log" 2>/dev/null | head -5
    
    echo ""
    echo "=== Checking Milvus logs for CBO initialization ==="
    echo "Looking for CBO logger initialization message..."
    if docker exec "$CONTAINER_NAME" test -d "$WORK_DIR/logs" 2>/dev/null; then
        INIT_MSG=$(docker exec "$CONTAINER_NAME" grep -l "CBO Logger initialized" "$WORK_DIR/logs"/*.log 2>/dev/null | head -1)
        if [ -n "$INIT_MSG" ]; then
            echo "✓ Found CBO initialization in: $INIT_MSG"
            docker exec "$CONTAINER_NAME" grep "CBO Logger initialized" "$INIT_MSG" | tail -1
        else
            echo "⚠ CBO logger not yet initialized"
            echo "  This means no search queries with filters have been executed yet."
        fi
    else
        echo "⚠ Logs directory does not exist: $WORK_DIR/logs"
    fi
    
    echo ""
    echo "=== Summary ==="
    echo "The CBO log file will be created when:"
    echo "  1. Milvus is running ✓"
    echo "  2. A search query with filters is executed (triggering CBO)"
    echo ""
    echo "Expected locations (in order of priority):"
    echo "  1. If log.file.rootPath is configured: {rootPath}/cbo.log"
    echo "  2. Current working directory: $WORK_DIR/logs/cbo.log"
    echo "  3. Fallback: /tmp/milvus/logs/cbo.log"
    echo ""
    echo "To check Milvus logs for CBO initialization:"
    echo "  docker exec $CONTAINER_NAME grep -r 'CBO Logger initialized' $WORK_DIR/logs/ 2>/dev/null"
    echo ""
    echo "To check CBO decision logs:"
    echo "  docker exec $CONTAINER_NAME grep -r 'CBO Decision' $WORK_DIR/logs/ 2>/dev/null"
fi
