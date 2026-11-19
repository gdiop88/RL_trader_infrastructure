
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Systemd Service Deployment
# ==========================================

log "Preparing infrastructure service deployment"

# ==========================================
# Validate build artifacts
# ==========================================

[[ -n "${BUILD_DIR:-}" && -d "$BUILD_DIR" ]] ||
    fail "Temporary build directory not initialized"

[[ -x "$BUILD_DIR/$SERVICE" ]] ||
    fail "Compiled service executable not found"

[[ -x "$BUILD_DIR/${SERVICE}-migrate" ]] ||
    fail "Compiled migration executable not found"

[[ -f "$SOURCE_CONFIG" ]] ||
    fail "Source configuration not found"

[[ -f "$SOURCE_UNIT" ]] ||
    fail "Systemd unit template not found"

# ==========================================
# Validate service identity
# ==========================================

getent group "$SERVICE_GROUP" >/dev/null 2>&1 ||
    fail "Service group does not exist: $SERVICE_GROUP"

id "$SERVICE_USER" >/dev/null 2>&1 ||
    fail "Service user does not exist: $SERVICE_USER"

[[ "$(id -gn "$SERVICE_USER")" == "$SERVICE_GROUP" ]] ||
    fail "Service user has an unexpected primary group"

# ==========================================
# Create installation directories
# ==========================================

log "Preparing installation directories"

install -d \
    -o root \
    -g root \
    -m 0755 \
    "$BIN_DIR"

install -d \
    -o root \
    -g "$SERVICE_GROUP" \
    -m 0750 \
    "$CONFIG_DIR"

install -d \
    -o "$SERVICE_USER" \
    -g "$SERVICE_GROUP" \
    -m 0750 \
    "$DATA_DIR"

# ==========================================
# Install configuration
# ==========================================

if [[ ! -f "$TARGET_CONFIG" ]]; then
    log "Installing service configuration"

    install \
        -o root \
        -g "$SERVICE_GROUP" \
        -m 0640 \
        "$SOURCE_CONFIG" \
        "$TARGET_CONFIG"
else
    log "Preserving existing configuration: $TARGET_CONFIG"

    if ! cmp -s "$SOURCE_CONFIG" "$TARGET_CONFIG"; then
        warn "Installed configuration differs from configs/server.yaml; review $TARGET_CONFIG"
    fi
fi

# ==========================================
# Install migration tool and SQL sources
# ==========================================

log "Installing database migration tooling"

MIGRATION_BIN="$BIN_DIR/${SERVICE}-migrate"
MIGRATION_DIR="$CONFIG_DIR/migrations"

install -d \
    -o root \
    -g "$SERVICE_GROUP" \
    -m 0750 \
    "$MIGRATION_DIR"

for migration in "$PROJECT_DIR"/migrations/*.sql; do
    [[ -f "$migration" ]] ||
        fail "No SQL migrations found"

    install \
        -o root \
        -g "$SERVICE_GROUP" \
        -m 0640 \
        "$migration" \
        "$MIGRATION_DIR/$(basename "$migration")"
done

install \
    -o root \
    -g root \
    -m 0755 \
    "$BUILD_DIR/${SERVICE}-migrate" \
    "$MIGRATION_BIN"

# ==========================================
# Verify Aiven migration state
# ==========================================

log "Checking Aiven database migration state"

if ! migration_state="$("$MIGRATION_BIN" \
    -action=version \
    -config="$TARGET_CONFIG" \
    -migrations="$MIGRATION_DIR" 2>&1)"; then

    printf '%s\n' "$migration_state" >&2
    fail "Database migration check failed; existing service not stopped"
fi

printf '%s\n' "$migration_state"

if ! grep -Eq \
    '^Migration version: [1-9][0-9]*$' \
    <<< "$migration_state"; then

    fail "Database has no applied migration; apply explicitly before deployment"
fi

if ! grep -q '^Dirty: false$' <<< "$migration_state"; then
    fail "Database migration state is dirty"
fi

# ==========================================
# Generate systemd unit
# ==========================================

log "Generating systemd unit"

GENERATED_UNIT="$BUILD_DIR/$SERVICE.service"

sed \
    -e "s|{{SERVICE_USER}}|$SERVICE_USER|g" \
    -e "s|{{SERVICE_GROUP}}|$SERVICE_GROUP|g" \
    -e "s|{{BIN_PATH}}|$TARGET_BIN|g" \
    -e "s|{{CONFIG_PATH}}|$TARGET_CONFIG|g" \
    -e "s|{{DATA_DIR}}|$DATA_DIR|g" \
    "$SOURCE_UNIT" > "$GENERATED_UNIT"

# ==========================================
# Validate generated unit
# ==========================================

if grep -q '{{' "$GENERATED_UNIT"; then
    fail "Unresolved systemd template variables"
fi

log "Validating systemd unit"

systemd-analyze verify "$GENERATED_UNIT" ||
    fail "Systemd unit validation failed"

# ==========================================
# Stop existing service
# ==========================================

if systemctl is-active --quiet "$SERVICE.service"; then
    log "Stopping existing service"
    systemctl stop "$SERVICE.service"
fi

# ==========================================
# Install executable
# ==========================================

log "Installing infrastructure executable"

install \
    -o root \
    -g root \
    -m 0755 \
    "$BUILD_DIR/$SERVICE" \
    "$TARGET_BIN"

# ==========================================
# Install systemd unit
# ==========================================

log "Installing systemd unit"

install -d \
    -o root \
    -g root \
    -m 0755 \
    "$UNIT_DIR"

install \
    -o root \
    -g root \
    -m 0644 \
    "$GENERATED_UNIT" \
    "$TARGET_UNIT"

# ==========================================
# Reload systemd
# ==========================================

log "Reloading systemd"

systemctl daemon-reload

# ==========================================
# Enable service
# ==========================================

log "Enabling infrastructure service"

systemctl enable "$SERVICE.service"

# ==========================================
# Start service
# ==========================================

log "Starting infrastructure service"

systemctl restart "$SERVICE.service"

log "Systemd deployment completed"
