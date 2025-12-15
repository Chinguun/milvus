#!/bin/bash

# Build Milvus GPU version
echo "Building Milvus GPU version..."
docker exec milvus-gpubuilder-1 bash -c "cd /go/src/github.com/milvus-io/milvus && make milvus-gpu"

# Check if build succeeded
if docker exec milvus-gpubuilder-1 test -f /go/src/github.com/milvus-io/milvus/bin/milvus; then
    echo "Build successful! Starting Milvus standalone..."
    
    # Run Milvus standalone
    docker exec milvus-gpubuilder-1 bash -c "cd /go/src/github.com/milvus-io/milvus && source scripts/setenv.sh && export MILVUS_DIR=/go/src/github.com/milvus-io/milvus && ./bin/milvus run standalone"
else
    echo "Build failed! Binary not found."
    exit 1
fi

