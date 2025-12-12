#!/bin/bash
# Script to locate CBO log file
# NOTE: If Milvus is running inside Docker, use find_cbo_log_container.sh instead

echo "Searching for CBO log file (cbo.log)..."
echo ""
echo "⚠ NOTE: If Milvus is running inside Docker, use: ./find_cbo_log_container.sh"
echo ""

# Check common locations
LOCATIONS=(
    "/tmp/milvus/logs/cbo.log"
    "$(pwd)/logs/cbo.log"
    "$HOME/milvus/logs/cbo.log"
    "/var/lib/milvus/logs/cbo.log"
)

FOUND=false

for loc in "${LOCATIONS[@]}"; do
    if [ -f "$loc" ]; then
        echo "✓ Found CBO log file at: $loc"
        echo "  Size: $(ls -lh "$loc" | awk '{print $5}')"
        echo "  Last modified: $(stat -c '%y' "$loc" 2>/dev/null || stat -f '%Sm' "$loc" 2>/dev/null)"
        echo ""
        echo "To view the log file:"
        echo "  tail -f $loc"
        echo "  cat $loc"
        FOUND=true
        break
    fi
done

if [ "$FOUND" = false ]; then
    echo "⚠ CBO log file not found in common locations."
    echo ""
    echo "The CBO log file will be created when:"
    echo "  1. Milvus is running"
    echo "  2. A search query with filters is executed (triggering CBO)"
    echo ""
    echo "Searching all filesystems for cbo.log..."
    echo "(This may take a while...)"
    echo ""
    
    # Try to find it anywhere
    RESULT=$(find /tmp /var /home -name "cbo.log" 2>/dev/null | head -5)
    if [ -n "$RESULT" ]; then
        echo "Found potential CBO log files:"
        echo "$RESULT"
    else
        echo "No cbo.log files found. The CBO logger will create the file at:"
        echo "  - If log.file.rootPath is configured: {rootPath}/cbo.log"
        echo "  - Otherwise: {current_working_directory}/logs/cbo.log"
        echo "  - Fallback: /tmp/milvus/logs/cbo.log"
        echo ""
        echo "Check Milvus startup logs for: '🔥 CBO Logger initialized - CBO logs will be written to dedicated file'"
    fi
fi

