# Milvus Docker Development Environment Setup Guide

This guide will help you set up a Docker-based development environment for building Milvus.

## Prerequisites

- **Docker**: Version 19.03 or higher
- **Docker Compose**: Version 1.25.1 or higher (or Docker Compose V2)
- **Hardware**: 
  - 8GB RAM minimum (16GB recommended)
  - 50GB free disk space
  - CPU with SIMD support (SSE4.2, AVX, AVX2, or AVX512)

### Verify Prerequisites

```bash
docker --version
docker compose version
```

## Quick Start

### Option 1: Using the Setup Script (Recommended)

```bash
# Setup the environment
./setup-dev-docker.sh setup

# Start dev containers
./setup-dev-docker.sh devcontainer

# Build Milvus
./setup-dev-docker.sh build

# Or open a shell in the builder container
./setup-dev-docker.sh shell
```

### Option 2: Manual Setup

#### 1. Start Development Containers

This starts all required services (etcd, MinIO, Pulsar, etc.) and the builder container:

```bash
./scripts/devcontainer.sh up
```

#### 2. Enter the Builder Container

```bash
docker exec -ti milvus_builder_1 bash
```

#### 3. Build Milvus

Inside the container:

```bash
make milvus
```

Or from the host:

```bash
./build/builder.sh make
```

#### 4. Build Milvus Docker Image

```bash
./build/build_image.sh
```

## Common Workflows

### Building Milvus Binary

```bash
# Using builder script (recommended)
./build/builder.sh make

# Or specify OS
export OS_NAME=ubuntu22.04
./build/builder.sh make
```

The binary will be at `bin/milvus`.

### Running Unit Tests

```bash
# Start dependencies first
cd deployments/docker/dev
docker compose up -d
cd ../../..

# Run tests
./build/builder.sh make unittest
```

### Building Docker Image

```bash
# Build CPU image
./build/build_image.sh

# Build GPU image
./build/build_image_gpu.sh

# Specify OS
export OS_NAME=ubuntu22.04
./build/build_image.sh
```

### Using VS Code Dev Container

1. Install [Remote Development extension pack](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.vscode-remote-extensionpack)
2. Open VS Code in the Milvus directory
3. Press `F1` and select "Remote-Containers: Open Folder in Container"
4. VS Code will build and connect to the dev container automatically

## Environment Variables

You can customize the build by setting these environment variables:

- `OS_NAME`: OS for builder (default: `ubuntu20.04`)
  - Options: `ubuntu20.04`, `ubuntu22.04`, `amazonlinux2023`, `rockylinux8`
- `IMAGE_ARCH`: Architecture (default: `amd64`)
  - Options: `amd64`, `arm64`
- `IMAGE_REPO`: Docker image repository (default: `milvusdb`)
- `DOCKER_VOLUME_DIRECTORY`: Directory for Docker volumes (default: `.docker`)

Example:

```bash
export OS_NAME=ubuntu22.04
export IMAGE_ARCH=amd64
./build/builder.sh make
```

## Directory Structure

```
milvus/
├── build/
│   ├── builder.sh          # Run commands in builder container
│   ├── build_image.sh      # Build Milvus Docker image
│   └── docker/
│       ├── builder/        # Builder Dockerfiles
│       └── milvus/         # Milvus Dockerfiles
├── scripts/
│   └── devcontainer.sh     # Dev container management
├── deployments/docker/dev/ # Development docker-compose
├── docker-compose.yml      # Main docker-compose for CI/builds
└── .devcontainer.json      # VS Code dev container config
```

## Docker Volumes

The setup creates volumes for caching:

- `.docker/{arch}-{os}-ccache`: C++ compiler cache
- `.docker/{arch}-{os}-go-mod`: Go module cache
- `.docker/{arch}-{os}-conan`: Conan package cache
- `.docker/{arch}-{os}-vscode-extensions`: VS Code extensions

These volumes speed up subsequent builds.

## Troubleshooting

### Builder container fails to start

```bash
# Check Docker logs
docker compose -f docker-compose-devcontainer.yml logs builder

# Rebuild builder image
CHECK_BUILDER=1 ./scripts/devcontainer.sh build
```

### Build fails with "library not found"

Ensure you're running commands inside the builder container or using `build/builder.sh`:

```bash
./build/builder.sh make
```

### Out of disk space

Clean up Docker volumes:

```bash
./setup-dev-docker.sh clean
```

Or manually:

```bash
docker system prune -a --volumes
```

### Permission issues

Ensure Docker volumes have correct permissions:

```bash
chmod -R 777 .docker
```

### Network issues

If you're behind a proxy, configure Docker:

```bash
# Create/edit ~/.docker/config.json
{
  "proxies": {
    "default": {
      "httpProxy": "http://proxy.example.com:8080",
      "httpsProxy": "http://proxy.example.com:8080"
    }
  }
}
```

## Stopping Containers

```bash
# Stop dev containers
./scripts/devcontainer.sh down

# Stop all Milvus-related containers
docker compose -f docker-compose-devcontainer.yml down
```

## Next Steps

- Read [DEVELOPMENT.md](DEVELOPMENT.md) for detailed development guidelines
- Check [build/README.md](build/README.md) for more Docker build options
- Review [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines

## Additional Resources

- [Milvus Documentation](https://milvus.io/docs)
- [Docker Documentation](https://docs.docker.com/)
- [VS Code Remote Containers](https://code.visualstudio.com/docs/remote/containers)

