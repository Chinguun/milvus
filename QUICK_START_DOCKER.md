# Quick Start: Building Milvus with Docker

## 🚀 Fastest Way to Get Started

```bash
# 1. Setup environment
./setup-dev-docker.sh setup

# 2. Start dev containers (etcd, MinIO, Pulsar, etc.)
./setup-dev-docker.sh devcontainer

# 3. Build Milvus
./build/builder.sh make
```

## 📋 Common Commands

| Task | Command |
|------|---------|
| **Setup** | `./setup-dev-docker.sh setup` |
| **Start containers** | `./setup-dev-docker.sh devcontainer` |
| **Build binary** | `./build/builder.sh make` |
| **Build Docker image** | `./build/build_image.sh` |
| **Run tests** | `./build/builder.sh make unittest` |
| **Open shell** | `./build/builder.sh /bin/bash` |
| **Stop containers** | `./scripts/devcontainer.sh down` |
| **Clean up** | `./setup-dev-docker.sh clean` |

## 🐳 Inside Builder Container

Once containers are running:

```bash
# Enter builder container
docker exec -ti milvus_builder_1 bash

# Inside container:
make milvus          # Build Milvus
make unittest        # Run tests
make verifiers       # Run all checks
make clean           # Clean build artifacts
```

## 🔧 Customization

```bash
# Use different OS
export OS_NAME=ubuntu20.04
./build/builder.sh make

# Use different architecture
export IMAGE_ARCH=arm64
./build/builder.sh make
```

## 📁 Output Locations

- **Binary**: `bin/milvus`
- **Libraries**: `lib/`
- **Docker image**: `milvusdb/milvus:latest`

## 🆘 Need Help?

- Full guide: See [DOCKER_DEV_SETUP.md](DOCKER_DEV_SETUP.md)
- Development guide: See [DEVELOPMENT.md](DEVELOPMENT.md)
- Build docs: See [build/README.md](build/README.md)

