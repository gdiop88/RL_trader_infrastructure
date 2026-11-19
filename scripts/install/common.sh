
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Shared Configuration and Utilities
# ==========================================

# ==========================================
# Service configuration
# ==========================================

SERVICE="${SERVICE:-infrastructure}"
SERVICE_USER="${SERVICE_USER:-infrastructure}"
SERVICE_GROUP="${SERVICE_GROUP:-infrastructure}"

# ==========================================
# Installation directories
# ==========================================

BIN_DIR="${BIN_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/$SERVICE}"
DATA_DIR="${DATA_DIR:-/var/lib/$SERVICE}"
UNIT_DIR="${UNIT_DIR:-/etc/systemd/system}"

# ==========================================
# Project-local tools
# ==========================================

TOOLS_DIR="$PROJECT_DIR/.tools"
TOOLS_BIN="$TOOLS_DIR/bin"
TOOLS_INCLUDE="$TOOLS_DIR/include"

# ==========================================
# Toolchain versions
# ==========================================

PROTOC_VERSION="${PROTOC_VERSION:-33.1}"
PROTOC_GEN_GO_VERSION="${PROTOC_GEN_GO_VERSION:-v1.36.11}"
PROTOC_GEN_GO_GRPC_VERSION="${PROTOC_GEN_GO_GRPC_VERSION:-v1.5.1}"

# ==========================================
# Source files
# ==========================================

SOURCE_CONFIG="$PROJECT_DIR/configs/server.yaml"
SOURCE_UNIT="$PROJECT_DIR/deployments/systemd/infrastructure.service"
SOURCE_PROTO="$PROJECT_DIR/api/proto/infrastructure.proto"

# ==========================================
# Installation targets
# ==========================================

TARGET_BIN="$BIN_DIR/$SERVICE"
TARGET_CONFIG="$CONFIG_DIR/server.yaml"
TARGET_UNIT="$UNIT_DIR/$SERVICE.service"

# ==========================================
# Go environment
# ==========================================

export PATH="$TOOLS_BIN:$PATH"
export GOBIN="$TOOLS_BIN"
export GOCACHE="$PROJECT_DIR/.cache/go-build"
export GOPATH="$PROJECT_DIR/.cache/go"

# ==========================================
# Temporary build directory
# ==========================================

BUILD_DIR=""

# ==========================================
# Logging
# ==========================================

log() {
    printf '[install] %s\n' "$*"
}

warn() {
    printf '[install] WARNING: %s\n' "$*" >&2
}

fail() {
    printf '[install] ERROR: %s\n' "$*" >&2
    exit 1
}

# ==========================================
# Cleanup
# ==========================================

cleanup() {
    if [[ -n "${BUILD_DIR:-}" && -d "$BUILD_DIR" ]]; then
        rm -rf -- "$BUILD_DIR"
    fi
}

# ==========================================
# Initialization
# ==========================================

log "Project: $PROJECT_DIR"
log "Service: $SERVICE"
