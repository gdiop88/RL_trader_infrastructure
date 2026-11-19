
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Protocol Buffers Toolchain
# ==========================================

log "Checking Protocol Buffers toolchain"

# ==========================================
# Install protoc
# ==========================================

install_protoc() {
    local arch
    local archive

    case "$(uname -m)" in
        x86_64)
            arch="linux-x86_64"
            ;;
        aarch64)
            arch="linux-aarch_64"
            ;;
        *)
            fail "Unsupported protoc architecture: $(uname -m)"
            ;;
    esac

    archive="$BUILD_DIR/protoc.zip"

    log "Downloading protoc $PROTOC_VERSION"

    curl \
        --fail \
        --location \
        --retry 3 \
        "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/protoc-${PROTOC_VERSION}-${arch}.zip" \
        --output "$archive"

    unzip -oq "$archive" \
        "bin/protoc" \
        "include/*" \
        -d "$TOOLS_DIR"

    chmod 0755 "$TOOLS_BIN/protoc"

    [[ -x "$TOOLS_BIN/protoc" ]] ||
        fail "protoc installation failed"
}

# ==========================================
# Check protoc version
# ==========================================

EXPECTED_PROTOC_VERSION="libprotoc $PROTOC_VERSION"

if [[ ! -x "$TOOLS_BIN/protoc" ]] ||
   [[ "$("$TOOLS_BIN/protoc" --version)" != "$EXPECTED_PROTOC_VERSION" ]]; then

    install_protoc

else
    log "protoc $PROTOC_VERSION already installed"
fi

# ==========================================
# Install Go generators
# ==========================================

install_go_generator() {
    local package="$1"
    local executable="$2"
    local version="$3"

    log "Installing $executable $version"

    (
        cd "$PROJECT_DIR"
        go install "${package}@${version}"
    )

    [[ -x "$TOOLS_BIN/$executable" ]] ||
        fail "$executable installation failed"
}

# ==========================================
# protoc-gen-go
# ==========================================

EXPECTED_GO_VERSION="protoc-gen-go $PROTOC_GEN_GO_VERSION"

if [[ ! -x "$TOOLS_BIN/protoc-gen-go" ]] ||
   [[ "$("$TOOLS_BIN/protoc-gen-go" --version)" != "$EXPECTED_GO_VERSION" ]]; then

    install_go_generator \
        "google.golang.org/protobuf/cmd/protoc-gen-go" \
        "protoc-gen-go" \
        "$PROTOC_GEN_GO_VERSION"

else
    log "protoc-gen-go already installed"
fi

# ==========================================
# protoc-gen-go-grpc
# ==========================================

# The gRPC generator reports its version without
# the leading "v", unlike protoc-gen-go.

GRPC_GENERATOR_VERSION="${PROTOC_GEN_GO_GRPC_VERSION#v}"

EXPECTED_GRPC_VERSION="protoc-gen-go-grpc $GRPC_GENERATOR_VERSION"

if [[ ! -x "$TOOLS_BIN/protoc-gen-go-grpc" ]] ||
   [[ "$("$TOOLS_BIN/protoc-gen-go-grpc" --version)" != "$EXPECTED_GRPC_VERSION" ]]; then

    install_go_generator \
        "google.golang.org/grpc/cmd/protoc-gen-go-grpc" \
        "protoc-gen-go-grpc" \
        "$PROTOC_GEN_GO_GRPC_VERSION"

else
    log "protoc-gen-go-grpc already installed"
fi

# ==========================================
# Verify installed toolchain
# ==========================================

log "Verifying Protocol Buffers toolchain"

"$TOOLS_BIN/protoc" --version
"$TOOLS_BIN/protoc-gen-go" --version
"$TOOLS_BIN/protoc-gen-go-grpc" --version

# ==========================================
# Generate Go bindings
# ==========================================

log "Generating Protobuf Go bindings"

(
    cd "$PROJECT_DIR"

    "$TOOLS_BIN/protoc" \
        --proto_path=api/proto \
        --plugin=protoc-gen-go="$TOOLS_BIN/protoc-gen-go" \
        --plugin=protoc-gen-go-grpc="$TOOLS_BIN/protoc-gen-go-grpc" \
        --go_out=api/proto \
        --go_opt=paths=source_relative \
        --go-grpc_out=api/proto \
        --go-grpc_opt=paths=source_relative \
        infrastructure.proto
)

# ==========================================
# Verify generated files
# ==========================================

GENERATED_PROTO="$PROJECT_DIR/api/proto/infrastructure.pb.go"
GENERATED_GRPC="$PROJECT_DIR/api/proto/infrastructure_grpc.pb.go"

[[ -s "$GENERATED_PROTO" ]] ||
    fail "Protobuf message bindings were not generated"

[[ -s "$GENERATED_GRPC" ]] ||
    fail "gRPC service bindings were not generated"

log "Protobuf Go bindings generated successfully"
