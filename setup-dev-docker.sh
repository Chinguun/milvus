#!/usr/bin/env bash

# Setup script for Milvus Docker development environment
# This script helps prepare the development environment for building Milvus with Docker

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "=========================================="
echo "Milvus Docker Development Environment Setup"
echo "=========================================="
echo ""

# Check Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    echo "   See: https://docs.docker.com/get-docker/"
    exit 1
fi

if ! docker info &> /dev/null; then
    echo "❌ Docker daemon is not running. Please start Docker first."
    exit 1
fi

echo "✅ Docker is installed and running"
echo ""

# Set default OS_NAME if not set
export OS_NAME="${OS_NAME:-ubuntu22.04}"
export IMAGE_ARCH="${IMAGE_ARCH:-amd64}"

echo "Configuration:"
echo "  OS_NAME: ${OS_NAME}"
echo "  IMAGE_ARCH: ${IMAGE_ARCH}"
echo ""

# Create necessary directories for Docker volumes
echo "Creating Docker volume directories..."
mkdir -p ".docker/${IMAGE_ARCH}-${OS_NAME}-ccache"
mkdir -p ".docker/${IMAGE_ARCH}-${OS_NAME}-go-mod"
mkdir -p ".docker/${IMAGE_ARCH}-${OS_NAME}-vscode-extensions"
mkdir -p ".docker/${IMAGE_ARCH}-${OS_NAME}-conan"
chmod -R 777 ".docker" 2>/dev/null || true
echo "✅ Volume directories created"
echo ""

# Function to show usage
show_usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  setup          - Setup development environment (default)"
    echo "  devcontainer   - Start dev container environment"
    echo "  build          - Build Milvus using Docker builder"
    echo "  image          - Build Milvus Docker image"
    echo "  test           - Run unit tests in Docker"
    echo "  shell          - Open shell in builder container"
    echo "  clean          - Clean up Docker volumes and containers"
    echo "  help           - Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 setup                    # Setup environment"
    echo "  $0 devcontainer             # Start dev containers"
    echo "  $0 build                    # Build Milvus"
    echo "  $0 shell                    # Open shell in builder"
    echo "  ./build/builder.sh make     # Build Milvus binary"
    echo "  ./build/build_image.sh      # Build Milvus Docker image"
    echo ""
}

# Function to start dev container
start_devcontainer() {
    echo "Starting dev container environment..."
    echo "This will start:"
    echo "  - Builder container (for building Milvus)"
    echo "  - etcd (metadata store)"
    echo "  - MinIO (object storage)"
    echo "  - Pulsar (message queue)"
    echo "  - Azurite (Azure storage emulator)"
    echo "  - GCP Native (GCS emulator)"
    echo ""
    
    ./scripts/devcontainer.sh up
    
    echo ""
    echo "✅ Dev containers started!"
    echo ""
    echo "To enter the builder container:"
    echo "  docker exec -ti milvus_builder_1 bash"
    echo ""
    echo "To stop the containers:"
    echo "  ./scripts/devcontainer.sh down"
    echo ""
}

# Function to build Milvus
build_milvus() {
    echo "Building Milvus using Docker builder..."
    echo "This may take a while on first run..."
    echo ""
    
    ./build/builder.sh make
    
    echo ""
    echo "✅ Build complete! Binary is at: bin/milvus"
    echo ""
}

# Function to build Docker image
build_image() {
    echo "Building Milvus Docker image..."
    echo ""
    
    ./build/build_image.sh
    
    echo ""
    echo "✅ Docker image built!"
    echo "  Image: milvusdb/milvus:latest"
    echo ""
    echo "To verify:"
    echo "  docker images | grep milvus"
    echo ""
}

# Function to run tests
run_tests() {
    echo "Running unit tests in Docker..."
    echo ""
    
    ./build/builder.sh make unittest
    
    echo ""
    echo "✅ Tests completed!"
    echo ""
}

# Function to open shell
open_shell() {
    echo "Opening shell in builder container..."
    echo ""
    
    ./build/builder.sh /bin/bash
}

# Function to clean up
cleanup() {
    echo "Cleaning up Docker volumes and containers..."
    echo ""
    
    read -p "This will remove all dev containers and volumes. Continue? (y/N) " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled."
        exit 0
    fi
    
    ./scripts/devcontainer.sh down
    
    echo ""
    echo "Removing volumes..."
    docker volume ls | grep milvus | awk '{print $2}' | xargs -r docker volume rm || true
    rm -rf .docker/* || true
    
    echo "✅ Cleanup complete!"
    echo ""
}

# Main command handling
case "${1:-setup}" in
    setup)
        echo "Setting up development environment..."
        echo ""
        echo "Next steps:"
        echo "  1. Start dev containers:  $0 devcontainer"
        echo "  2. Build Milvus:         $0 build"
        echo "  3. Or open shell:        $0 shell"
        echo ""
        echo "For more options, run: $0 help"
        ;;
    devcontainer)
        start_devcontainer
        ;;
    build)
        build_milvus
        ;;
    image)
        build_image
        ;;
    test)
        run_tests
        ;;
    shell)
        open_shell
        ;;
    clean)
        cleanup
        ;;
    help|--help|-h)
        show_usage
        ;;
    *)
        echo "Unknown command: $1"
        echo ""
        show_usage
        exit 1
        ;;
esac

