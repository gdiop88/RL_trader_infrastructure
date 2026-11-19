
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Go Dependencies and Build
# ==========================================

log "Starting Go build process"

# ==========================================
# Validate build environment
# ==========================================

[[ -d "$PROJECT_DIR" ]] ||
    fail "Project directory not found: $PROJECT_DIR"

[[ -f "$PROJECT_DIR/go.mod" ]] ||
    fail "Go module not found"

[[ -d "$BUILD_DIR" ]] ||
    fail "Temporary build directory not initialized"

[[ -d "$PROJECT_DIR/cmd/server" ]] ||
    fail "Go server source directory not found"

[[ -f "$PROJECT_DIR/cmd/migrate/main.go" ]] ||
    fail "Migration command source not found"

[[ -f "$PROJECT_DIR/migrations/000001_initial_schema.up.sql" ]] ||
    fail "Initial database migration not found"

# ==========================================
# Resolve Go dependencies
# ==========================================

log "Resolving Go dependencies"

(
    cd "$PROJECT_DIR"
    go mod tidy
)

# ==========================================
# Verify Go module dependencies
# ==========================================

log "Verifying Go dependencies"

(
    cd "$PROJECT_DIR"
    go mod verify
)

# ==========================================
# Compile infrastructure service
# ==========================================

log "Building infrastructure service"

(
    cd "$PROJECT_DIR"

    CGO_ENABLED=0 go build \
        -trimpath \
        -buildvcs=false \
        -o "$BUILD_DIR/$SERVICE" \
        ./cmd/server
)

# ==========================================
# Compile migration command
# ==========================================

log "Building database migration command"

(
    cd "$PROJECT_DIR"

    CGO_ENABLED=0 go build \
        -trimpath \
        -buildvcs=false \
        -o "$BUILD_DIR/${SERVICE}-migrate" \
        ./cmd/migrate
)

# ==========================================
# Verify compiled executables
# ==========================================

BUILD_BINARY="$BUILD_DIR/$SERVICE"

[[ -f "$BUILD_BINARY" ]] ||
    fail "Compiled service executable not found"

[[ -x "$BUILD_BINARY" ]] ||
    fail "Compiled service executable is not executable"

[[ -x "$BUILD_DIR/${SERVICE}-migrate" ]] ||
    fail "Compiled migration executable is missing or not executable"

log "Go build completed successfully"
log "Executable: $BUILD_BINARY"
log "Migration executable: $BUILD_DIR/${SERVICE}-migrate"
