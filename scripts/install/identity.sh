
#!/usr/bin/env bash

# ==========================================
# Infrastructure Service Identity
# ==========================================
# Sourced by scripts/install.sh.
# Requires common.sh and prerequisites.sh.

set -Eeuo pipefail

log "Configuring infrastructure service identity"

# ==========================================
# Validate required configuration
# ==========================================

: "${SERVICE_USER:?SERVICE_USER is required}"
: "${SERVICE_GROUP:?SERVICE_GROUP is required}"

# ==========================================
# Create service group
# ==========================================

if getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
    log "Service group already exists: $SERVICE_GROUP"
else
    log "Creating service group: $SERVICE_GROUP"

    groupadd --system "$SERVICE_GROUP" ||
        fail "Failed to create service group: $SERVICE_GROUP"
fi

# ==========================================
# Create service user
# ==========================================

if id "$SERVICE_USER" >/dev/null 2>&1; then
    log "Service user already exists: $SERVICE_USER"
else
    log "Creating service user: $SERVICE_USER"

    useradd \
        --system \
        --gid "$SERVICE_GROUP" \
        --home-dir "$DATA_DIR" \
        --shell /usr/sbin/nologin \
        --no-create-home \
        "$SERVICE_USER" ||
        fail "Failed to create service user: $SERVICE_USER"
fi

# ==========================================
# Validate service identity
# ==========================================

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
    fail "Service user does not exist: $SERVICE_USER"
fi

if ! getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
    fail "Service group does not exist: $SERVICE_GROUP"
fi

# Ensure the service user has the configured
# primary group without silently modifying
# an existing account.
actual_group="$(id -gn "$SERVICE_USER")"

if [[ "$actual_group" != "$SERVICE_GROUP" ]]; then
    fail "Service user $SERVICE_USER has primary group $actual_group; expected $SERVICE_GROUP"
fi

log "Service identity configured successfully"
log "User: $SERVICE_USER"
log "Group: $SERVICE_GROUP"
