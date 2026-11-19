
#!/usr/bin/env bash
set -Eeuo pipefail

# ==========================================
# Infrastructure Service Installer
# ==========================================

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
PROJECT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd -P)"
INSTALL_DIR="$SCRIPT_DIR/install"

# ==========================================
# Validate installation modules
# ==========================================

MODULES=(
    common.sh
    prerequisites.sh
    protobuf.sh
    build.sh
    identity.sh
    certificates.sh
    service.sh
    verify.sh
)

for module in "${MODULES[@]}"; do
    if [[ ! -f "$INSTALL_DIR/$module" ]]; then
        printf '[install] ERROR: Missing module: %s\n' \
            "$INSTALL_DIR/$module" >&2
        exit 1
    fi
done

# ==========================================
# Load shared configuration
# ==========================================

source "$INSTALL_DIR/common.sh"

trap cleanup EXIT

# ==========================================
# Execute installation
# ==========================================

source "$INSTALL_DIR/prerequisites.sh"
source "$INSTALL_DIR/protobuf.sh"
source "$INSTALL_DIR/build.sh"
source "$INSTALL_DIR/identity.sh"
source "$INSTALL_DIR/certificates.sh"
source "$INSTALL_DIR/service.sh"
source "$INSTALL_DIR/verify.sh"
