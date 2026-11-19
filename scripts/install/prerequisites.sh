
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Automatic Linux Dependency Provisioning
# ==========================================

log "Checking installation prerequisites"

# ==========================================
# Root privileges
# ==========================================

[[ "$EUID" -eq 0 ]] ||
    fail "Run the installer as root"

# ==========================================
# Detect Linux distribution
# ==========================================

DISTRO_ID="unknown"
DISTRO_LIKE=""

if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release

    DISTRO_ID="${ID:-unknown}"
    DISTRO_LIKE="${ID_LIKE:-}"
fi

log "Linux distribution: $DISTRO_ID"

# ==========================================
# Detect package manager
# ==========================================

detect_package_manager() {
    local manager

    for manager in \
        apt-get \
        dnf \
        yum \
        zypper \
        pacman \
        apk
    do
        if command -v "$manager" >/dev/null 2>&1; then
            printf '%s\n' "$manager"
            return 0
        fi
    done

    return 1
}

PACKAGE_MANAGER=""

if PACKAGE_MANAGER="$(detect_package_manager)"; then
    log "Package manager: $PACKAGE_MANAGER"
else
    warn "No supported package manager detected"
fi

# ==========================================
# Install system packages
# ==========================================

APT_UPDATED=false

install_packages() {
    [[ -n "$PACKAGE_MANAGER" ]] ||
        fail "No supported package manager available"

    local packages=("$@")

    log "Installing packages: ${packages[*]}"

    case "$PACKAGE_MANAGER" in
        apt-get)
            if [[ "$APT_UPDATED" == false ]]; then
                DEBIAN_FRONTEND=noninteractive \
                    apt-get update ||
                    fail "APT repository update failed"

                APT_UPDATED=true
            fi

            DEBIAN_FRONTEND=noninteractive \
                apt-get install \
                    -y \
                    --no-install-recommends \
                    "${packages[@]}" ||
                fail "APT package installation failed"
            ;;

        dnf)
            dnf install -y "${packages[@]}" ||
                fail "DNF package installation failed"
            ;;

        yum)
            yum install -y "${packages[@]}" ||
                fail "YUM package installation failed"
            ;;

        zypper)
            zypper --non-interactive install \
                "${packages[@]}" ||
                fail "Zypper package installation failed"
            ;;

        pacman)
            pacman -S \
                --noconfirm \
                --needed \
                "${packages[@]}" ||
                fail "Pacman package installation failed"
            ;;

        apk)
            apk add --no-cache "${packages[@]}" ||
                fail "APK package installation failed"
            ;;

        *)
            fail "Unsupported package manager: $PACKAGE_MANAGER"
            ;;
    esac
}

# ==========================================
# Map commands to distribution packages
# ==========================================

package_for() {
    local dependency="$1"

    case "$dependency" in

        go)
            case "$PACKAGE_MANAGER" in
                apt-get) echo golang-go ;;
                apk) echo go ;;
                *) echo golang ;;
            esac
            ;;

        curl)
            echo curl
            ;;

        unzip)
            echo unzip
            ;;

        openssl)
            echo openssl
            ;;

        getent)
            case "$PACKAGE_MANAGER" in
                apt-get) echo libc-bin ;;
                apk) echo musl-utils ;;
                pacman) echo glibc ;;
                zypper) echo glibc ;;
                *) echo glibc-common ;;
            esac
            ;;

        useradd|groupadd)
            case "$PACKAGE_MANAGER" in
                apt-get) echo passwd ;;
                apk) echo shadow ;;
                pacman) echo shadow ;;
                zypper) echo shadow ;;
                *) echo shadow-utils ;;
            esac
            ;;

        systemctl|systemd-analyze)
            echo systemd
            ;;

        stat|install|id|chmod|chown|mkdir|rm|cat|uname)
            echo coreutils
            ;;

        runuser)
            case "$PACKAGE_MANAGER" in
                apt-get) echo util-linux ;;
                apk) echo util-linux ;;
                *) echo util-linux ;;
            esac
            ;;

        mktemp)
            case "$PACKAGE_MANAGER" in
                apk) echo coreutils ;;
                *) echo coreutils ;;
            esac
            ;;

        sed)
            echo sed
            ;;

        grep)
            echo grep
            ;;

        *)
            return 1
            ;;
    esac
}

# ==========================================
# Ensure command is available
# ==========================================

ensure_command() {
    local command_name="$1"

    if command -v "$command_name" >/dev/null 2>&1; then
        return 0
    fi

    local package

    package="$(package_for "$command_name")" ||
        fail "No package mapping for: $command_name"

    install_packages "$package"

    if ! command -v "$command_name" >/dev/null 2>&1; then
        fail "Command unavailable after installation: $command_name"
    fi
}

# ==========================================
# Install missing dependencies
# ==========================================

log "Checking system dependencies"

REQUIRED_COMMANDS=(
    curl
    unzip
    openssl
    go
    getent
    useradd
    groupadd
    systemctl
    systemd-analyze
    stat
    runuser
    id
    install
    mktemp
    sed
    grep
    uname
    mkdir
    chmod
    chown
    rm
    cat
)

for cmd in "${REQUIRED_COMMANDS[@]}"; do
    ensure_command "$cmd"
done

log "All required system commands available"

# ==========================================
# Validate systemd runtime
# ==========================================

log "Checking systemd runtime"

[[ -d /run/systemd/system ]] ||
    fail "Systemd is not the active init system"

systemctl --version >/dev/null 2>&1 ||
    fail "Systemd is not operational"

# ==========================================
# Validate project structure
# ==========================================

log "Validating project structure"

[[ -f "$PROJECT_DIR/go.mod" ]] ||
    fail "Go module not found: $PROJECT_DIR/go.mod"

[[ -d "$PROJECT_DIR/cmd/server" ]] ||
    fail "Go server directory not found"

[[ -f "$SOURCE_CONFIG" ]] ||
    fail "Configuration not found: $SOURCE_CONFIG"

[[ -f "$SOURCE_UNIT" ]] ||
    fail "Systemd template not found: $SOURCE_UNIT"

[[ -f "$SOURCE_PROTO" ]] ||
    fail "Protobuf definition not found: $SOURCE_PROTO"

log "Project structure validated"

# ==========================================
# Validate service identifiers
# ==========================================

[[ "$SERVICE" =~ ^[a-zA-Z][a-zA-Z0-9_-]*$ ]] ||
    fail "Invalid service name: $SERVICE"

[[ "$SERVICE_USER" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]] ||
    fail "Invalid service user: $SERVICE_USER"

[[ "$SERVICE_GROUP" =~ ^[a-zA-Z_][a-zA-Z0-9_-]*$ ]] ||
    fail "Invalid service group: $SERVICE_GROUP"

# ==========================================
# Validate installation paths
# ==========================================

for path in \
    "$BIN_DIR" \
    "$CONFIG_DIR" \
    "$DATA_DIR" \
    "$UNIT_DIR"
do
    [[ "$path" == /* ]] ||
        fail "Installation path must be absolute: $path"

    [[ "$path" =~ ^/[a-zA-Z0-9_./-]+$ ]] ||
        fail "Unsupported installation path: $path"
done

# ==========================================
# Verify Go toolchain
# ==========================================

log "Checking Go toolchain"

REQUIRED_GO_VERSION="$(
    sed -n 's/^go[[:space:]]\+//p' \
        "$PROJECT_DIR/go.mod" | head -n 1
)"

[[ -n "$REQUIRED_GO_VERSION" ]] ||
    fail "Cannot determine Go version from go.mod"

INSTALLED_GO_VERSION="$(
    go version | sed -n 's/.* go\([0-9][0-9.]*\).*/\1/p'
)"

[[ -n "$INSTALLED_GO_VERSION" ]] ||
    fail "Cannot determine installed Go version"

log "Required Go version: $REQUIRED_GO_VERSION"
log "Installed Go version: $INSTALLED_GO_VERSION"

# Go's own toolchain management may download a newer
# compatible compiler when GOTOOLCHAIN=auto.
# Validate compatibility without replacing the
# system-installed compiler.

GOTOOLCHAIN=auto go version >/dev/null 2>&1 ||
    fail "Go toolchain validation failed"

# ==========================================
# Initialize local tool directories
# ==========================================

log "Initializing project tool directories"

mkdir -p \
    "$TOOLS_BIN" \
    "$TOOLS_INCLUDE" \
    "$GOCACHE" \
    "$GOPATH"

# ==========================================
# Initialize temporary build directory
# ==========================================

BUILD_DIR="$(mktemp -d)"

[[ -d "$BUILD_DIR" ]] ||
    fail "Failed to create temporary build directory"

# ==========================================
# Verify installed tools
# ==========================================

log "Go compiler: $(go version)"
log "OpenSSL: $(openssl version)"
log "Systemd: $(systemctl --version | head -n 1)"

# ==========================================
# Complete
# ==========================================

log "Prerequisites validated successfully"
