
#!/usr/bin/env bash

# ==========================================
# Infrastructure Installer
# Installation Verification
# ==========================================

log "Verifying infrastructure installation"

# ==========================================
# Verify installed executable
# ==========================================

log "Checking installed executable"

[[ -f "$TARGET_BIN" ]] ||
    fail "Service executable not found: $TARGET_BIN"

[[ -x "$TARGET_BIN" ]] ||
    fail "Service executable is not executable"

log "Service executable verified"

# ==========================================
# Verify installed configuration
# ==========================================

log "Checking service configuration"

[[ -f "$TARGET_CONFIG" ]] ||
    fail "Service configuration not found: $TARGET_CONFIG"

[[ -r "$TARGET_CONFIG" ]] ||
    fail "Service configuration is not readable"

log "Service configuration verified"

# ==========================================
# Verify TLS certificates
# ==========================================

log "Checking TLS certificates"

PKI_DIR="${CONFIG_DIR}/pki"
CERT_DIR="${CONFIG_DIR}/certs"

CA_KEY="${PKI_DIR}/ca.key"
CA_CERT="${PKI_DIR}/ca.crt"

SERVER_KEY="${CERT_DIR}/server.key"
SERVER_CERT="${CERT_DIR}/server.crt"

CLIENT_KEY="${PKI_DIR}/client.key"
CLIENT_CERT="${PKI_DIR}/client.crt"

RUNTIME_CA="${CERT_DIR}/ca.crt"

for file in \
    "$CA_KEY" \
    "$CA_CERT" \
    "$SERVER_KEY" \
    "$SERVER_CERT" \
    "$CLIENT_KEY" \
    "$CLIENT_CERT" \
    "$RUNTIME_CA"
do
    [[ -s "$file" ]] ||
        fail "Missing or empty TLS file: $file"
done

# ==========================================
# Verify certificate trust chains
# ==========================================

log "Verifying certificate trust chains"

openssl verify \
    -CAfile "$CA_CERT" \
    -purpose sslserver \
    "$SERVER_CERT" >/dev/null ||
    fail "Server certificate verification failed"

openssl verify \
    -CAfile "$CA_CERT" \
    -purpose sslclient \
    "$CLIENT_CERT" >/dev/null ||
    fail "Client certificate verification failed"

openssl verify \
    -CAfile "$CA_CERT" \
    "$RUNTIME_CA" >/dev/null ||
    fail "Runtime CA certificate verification failed"

# ==========================================
# Verify certificate expiration
# ==========================================

log "Checking certificate expiration"

for cert in \
    "$CA_CERT" \
    "$SERVER_CERT" \
    "$CLIENT_CERT"
do
    openssl x509 \
        -in "$cert" \
        -checkend 0 \
        -noout >/dev/null ||
        fail "Expired certificate: $cert"
done

# ==========================================
# Verify server certificate SAN
# ==========================================

log "Checking server certificate identity"

openssl x509 \
    -in "$SERVER_CERT" \
    -noout \
    -checkip 127.0.0.1 >/dev/null ||
    fail "Server certificate does not cover 127.0.0.1"

# ==========================================
# Verify certificate/private-key matching
# ==========================================

log "Checking certificate and private-key pairs"

verify_key_pair() {
    local cert="$1"
    local key="$2"
    local cert_pub
    local key_pub

    cert_pub="$(
        openssl x509 \
            -in "$cert" \
            -pubkey \
            -noout
    )" || fail "Cannot read certificate public key: $cert"

    key_pub="$(
        openssl pkey \
            -in "$key" \
            -pubout
    )" || fail "Cannot read private-key public key: $key"

    [[ "$cert_pub" == "$key_pub" ]] ||
        fail "Certificate/private-key mismatch: $cert"
}

verify_key_pair "$CA_CERT" "$CA_KEY"
verify_key_pair "$SERVER_CERT" "$SERVER_KEY"
verify_key_pair "$CLIENT_CERT" "$CLIENT_KEY"

# ==========================================
# Verify ownership and permissions
# ==========================================

log "Checking TLS file permissions"

check_owner_group() {
    local path="$1"
    local expected_user="$2"
    local expected_group="$3"

    local actual_user
    local actual_group

    actual_user="$(stat -c '%U' "$path")"
    actual_group="$(stat -c '%G' "$path")"

    [[ "$actual_user" == "$expected_user" ]] ||
        fail "Unexpected owner for $path: $actual_user"

    [[ "$actual_group" == "$expected_group" ]] ||
        fail "Unexpected group for $path: $actual_group"
}

check_mode() {
    local path="$1"
    local expected="$2"
    local actual

    actual="$(stat -c '%a' "$path")"

    [[ "$actual" == "$expected" ]] ||
        fail "Unexpected permissions for $path: $actual"
}

check_owner_group "$CA_KEY" root root
check_owner_group "$CLIENT_KEY" root root
check_owner_group "$SERVER_KEY" root "$SERVICE_GROUP"

check_mode "$PKI_DIR" 700
check_mode "$CA_KEY" 600
check_mode "$CLIENT_KEY" 600
check_mode "$SERVER_KEY" 640
check_mode "$CERT_DIR" 750

# ==========================================
# Verify runtime certificate access
# ==========================================

log "Checking service certificate access"

runuser -u "$SERVICE_USER" -- \
    test -r "$SERVER_KEY" ||
    fail "Service user cannot read server private key"

runuser -u "$SERVICE_USER" -- \
    test -r "$SERVER_CERT" ||
    fail "Service user cannot read server certificate"

runuser -u "$SERVICE_USER" -- \
    test -r "$RUNTIME_CA" ||
    fail "Service user cannot read trusted client CA"

if runuser -u "$SERVICE_USER" -- \
    test -r "$CA_KEY"; then
    fail "Service user can access CA private key"
fi

if runuser -u "$SERVICE_USER" -- \
    test -r "$CLIENT_KEY"; then
    fail "Service user can access client private key"
fi

log "TLS certificate verification successful"

# ==========================================
# Verify installed database migration tooling
# ==========================================

log "Checking installed database migration tooling"

MIGRATION_BIN="$BIN_DIR/${SERVICE}-migrate"
MIGRATION_DIR="$CONFIG_DIR/migrations"

[[ -x "$MIGRATION_BIN" ]] ||
    fail "Migration executable not found: $MIGRATION_BIN"

[[ -f "$MIGRATION_DIR/000001_initial_schema.up.sql" ]] ||
    fail "Initial SQL migration not installed"

migration_state="$("$MIGRATION_BIN" \
    -action=version \
    -config="$TARGET_CONFIG" \
    -migrations="$MIGRATION_DIR")" ||
    fail "Cannot verify database migration version"

printf '%s\n' "$migration_state"

grep -Eq \
    '^Migration version: [1-9][0-9]*$' \
    <<< "$migration_state" ||
    fail "Database migration version is not initialized"

grep -q '^Dirty: false$' <<< "$migration_state" ||
    fail "Database migration is dirty"

log "Database migration state verified"

# ==========================================
# Verify systemd unit
# ==========================================

log "Checking systemd unit"

[[ -f "$TARGET_UNIT" ]] ||
    fail "Systemd unit not found: $TARGET_UNIT"

systemd-analyze verify "$TARGET_UNIT" ||
    fail "Installed systemd unit validation failed"

log "Systemd unit verified"

# ==========================================
# Verify generated Protobuf bindings
# ==========================================

log "Checking generated Protobuf bindings"

[[ -s "$PROJECT_DIR/api/proto/infrastructure.pb.go" ]] ||
    fail "Missing Protobuf message bindings"

[[ -s "$PROJECT_DIR/api/proto/infrastructure_grpc.pb.go" ]] ||
    fail "Missing gRPC service bindings"

log "Protobuf bindings verified"

# ==========================================
# Verify service enabled
# ==========================================

log "Checking systemd enablement"

systemctl is-enabled --quiet "$SERVICE.service" ||
    fail "Service is not enabled"

log "Service enabled successfully"

# ==========================================
# Verify service running
# ==========================================

log "Checking service status"

if ! systemctl is-active --quiet "$SERVICE.service"; then
    systemctl status "$SERVICE.service" --no-pager || true
    fail "Infrastructure service is not running"
fi

log "Infrastructure service is active"

# ==========================================
# Verify service account
# ==========================================

log "Checking service account"

ACTUAL_USER="$(
    systemctl show "$SERVICE.service" \
        --property=User --value
)"

ACTUAL_GROUP="$(
    systemctl show "$SERVICE.service" \
        --property=Group --value
)"

MAIN_PID="$(
    systemctl show "$SERVICE.service" \
        --property=MainPID --value
)"

[[ "$ACTUAL_USER" == "$SERVICE_USER" ]] ||
    fail "Unexpected service user: $ACTUAL_USER"

[[ "$ACTUAL_GROUP" == "$SERVICE_GROUP" ]] ||
    fail "Unexpected service group: $ACTUAL_GROUP"

[[ "$MAIN_PID" =~ ^[0-9]+$ ]] ||
    fail "Invalid service process ID"

[[ "$MAIN_PID" -gt 0 ]] ||
    fail "Service has no running process"

log "Service account verified"
log "Service user: $ACTUAL_USER"
log "Service group: $ACTUAL_GROUP"
log "Main PID: $MAIN_PID"

# ==========================================
# Installation summary
# ==========================================

log "=========================================="
log "Infrastructure installation successful"
log "=========================================="

log "Service: $SERVICE.service"
log "Executable: $TARGET_BIN"
log "Configuration: $TARGET_CONFIG"
log "Systemd unit: $TARGET_UNIT"
log "Service user: $SERVICE_USER"
log "Service status: active"
log "TLS: certificates verified"
log "Database migrations: verified"

log "=========================================="
