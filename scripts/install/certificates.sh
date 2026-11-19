
#!/usr/bin/env bash

# Certificate provisioning for the infrastructure service.
# Intended to be sourced by scripts/install.sh.

set -euo pipefail

PKI_DIR="${CONFIG_DIR}/pki"
CERT_DIR="${CONFIG_DIR}/certs"

CA_KEY="${PKI_DIR}/ca.key"
CA_CERT="${PKI_DIR}/ca.crt"

SERVER_KEY="${CERT_DIR}/server.key"
SERVER_CERT="${CERT_DIR}/server.crt"

CLIENT_KEY="${PKI_DIR}/client.key"
CLIENT_CERT="${PKI_DIR}/client.crt"

log "Configuring infrastructure TLS certificates"

command -v openssl >/dev/null 2>&1 ||
    fail "OpenSSL is required for certificate provisioning"

install -d -m 700 "$PKI_DIR"
install -d -m 750 "$CERT_DIR"

# --------------------------------------------------
# Private certificate authority
# --------------------------------------------------

if [[ ! -f "$CA_KEY" && ! -f "$CA_CERT" ]]; then
    log "Generating private certificate authority"

    openssl req -x509 \
        -newkey rsa:4096 \
        -sha256 \
        -days 3650 \
        -noenc \
        -keyout "$CA_KEY" \
        -out "$CA_CERT" \
        -subj "/CN=RL Trader Infrastructure CA" \
        -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
        -addext "keyUsage=critical,keyCertSign,cRLSign"

elif [[ ! -f "$CA_KEY" || ! -f "$CA_CERT" ]]; then
    fail "Incomplete CA certificate/key pair"

else
    log "Existing certificate authority preserved"
fi

chmod 600 "$CA_KEY"
chmod 644 "$CA_CERT"

# --------------------------------------------------
# Server certificate
# --------------------------------------------------

if [[ ! -f "$SERVER_KEY" && ! -f "$SERVER_CERT" ]]; then
    log "Generating gRPC server certificate"

    openssl req \
        -new \
        -newkey rsa:3072 \
        -noenc \
        -keyout "$SERVER_KEY" \
        -out "${BUILD_DIR}/server.csr" \
        -subj "/CN=infrastructure"

    cat > "${BUILD_DIR}/server.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=serverAuth
subjectAltName=IP:127.0.0.1,IP:::1,DNS:localhost
EOF

    openssl x509 \
        -req \
        -in "${BUILD_DIR}/server.csr" \
        -CA "$CA_CERT" \
        -CAkey "$CA_KEY" \
        -CAcreateserial \
        -out "$SERVER_CERT" \
        -days 365 \
        -sha256 \
        -extfile "${BUILD_DIR}/server.ext"

elif [[ ! -f "$SERVER_KEY" || ! -f "$SERVER_CERT" ]]; then
    fail "Incomplete server certificate/key pair"

else
    log "Existing server certificate preserved"
fi

chmod 600 "$SERVER_KEY"
chmod 644 "$SERVER_CERT"

# --------------------------------------------------
# RL Trader CLI client certificate
# --------------------------------------------------

if [[ ! -f "$CLIENT_KEY" && ! -f "$CLIENT_CERT" ]]; then
    log "Generating RL Trader CLI client certificate"

    openssl req \
        -new \
        -newkey rsa:3072 \
        -noenc \
        -keyout "$CLIENT_KEY" \
        -out "${BUILD_DIR}/client.csr" \
        -subj "/CN=rl-trader-cli"

    cat > "${BUILD_DIR}/client.ext" <<'EOF'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature
extendedKeyUsage=clientAuth
EOF

    openssl x509 \
        -req \
        -in "${BUILD_DIR}/client.csr" \
        -CA "$CA_CERT" \
        -CAkey "$CA_KEY" \
        -CAcreateserial \
        -out "$CLIENT_CERT" \
        -days 365 \
        -sha256 \
        -extfile "${BUILD_DIR}/client.ext"

elif [[ ! -f "$CLIENT_KEY" || ! -f "$CLIENT_CERT" ]]; then
    fail "Incomplete client certificate/key pair"

else
    log "Existing client certificate preserved"
fi

chmod 600 "$CLIENT_KEY"
chmod 644 "$CLIENT_CERT"

# --------------------------------------------------
# Runtime certificate installation
# --------------------------------------------------

install -m 644 "$CA_CERT" "${CERT_DIR}/ca.crt"

chown root:root "$PKI_DIR"
chown root:root "$CA_KEY" "$CA_CERT"
chown root:root "$CLIENT_KEY" "$CLIENT_CERT"

chown root:"$SERVICE_GROUP" "$CERT_DIR"
chown root:"$SERVICE_GROUP" "$SERVER_KEY"
chown root:"$SERVICE_GROUP" "$SERVER_CERT"
chown root:"$SERVICE_GROUP" "${CERT_DIR}/ca.crt"

chmod 640 "$SERVER_KEY"
chmod 750 "$CERT_DIR"

# --------------------------------------------------
# Certificate verification
# --------------------------------------------------

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

openssl x509 \
    -in "$SERVER_CERT" \
    -checkend 0 \
    -noout >/dev/null ||
    fail "Server certificate has expired"

openssl x509 \
    -in "$CLIENT_CERT" \
    -checkend 0 \
    -noout >/dev/null ||
    fail "Client certificate has expired"

log "TLS certificate provisioning completed"
