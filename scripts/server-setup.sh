#!/bin/bash
# server-setup.sh
#
# One-time setup script to prepare your Linode Debian server for the weight app.
#
# Run as root or with sudo:
#   curl -fsSL https://raw.githubusercontent.com/exploded/weight/master/scripts/server-setup.sh | sudo bash
#
# After running, follow the printed instructions to add the SSH public key
# to your GitHub repository secrets and configure Caddy.

set -e

DEPLOY_USER="deploy"

echo "=== Weight App - Server Deployment Setup ==="
echo ""

# ---------------------------------------------------------------
# 1. Create deploy user (skip if already exists from moon setup)
# ---------------------------------------------------------------
if id "$DEPLOY_USER" &>/dev/null; then
    echo "[ok] User '$DEPLOY_USER' already exists"
else
    useradd -m -s /bin/bash "$DEPLOY_USER"
    echo "[ok] Created user '$DEPLOY_USER'"
fi

# ---------------------------------------------------------------
# 2. SSH key pair (reuse from moon if exists, otherwise generate)
# ---------------------------------------------------------------
KEY_DIR="/home/$DEPLOY_USER/.ssh"
KEY_FILE="$KEY_DIR/github_actions"

mkdir -p "$KEY_DIR"
chmod 700 "$KEY_DIR"

if [ ! -f "$KEY_FILE" ]; then
    ssh-keygen -t ed25519 -f "$KEY_FILE" -N "" -C "github-actions-deploy"
    echo "[ok] Generated SSH key pair at $KEY_FILE"
else
    echo "[ok] SSH key already exists at $KEY_FILE (reusing from existing setup)"
fi

if ! grep -qF "$(cat "$KEY_FILE.pub")" "$KEY_DIR/authorized_keys" 2>/dev/null; then
    cat "$KEY_FILE.pub" >> "$KEY_DIR/authorized_keys"
    echo "[ok] Public key added to authorized_keys"
fi

chmod 600 "$KEY_DIR/authorized_keys"
chown -R "$DEPLOY_USER:$DEPLOY_USER" "$KEY_DIR"

# ---------------------------------------------------------------
# 3. Create application directory
# ---------------------------------------------------------------
APP_DIR="/var/www/weight"
if [ -d "$APP_DIR" ]; then
    echo "[ok] Application directory $APP_DIR already exists"
else
    mkdir -p "$APP_DIR"
    chown www-data:www-data "$APP_DIR"
    echo "[ok] Created application directory $APP_DIR"
fi

# ---------------------------------------------------------------
# 4. Create .env template
# ---------------------------------------------------------------
ENV_FILE="$APP_DIR/.env"
if [ -f "$ENV_FILE" ]; then
    echo "[ok] .env file already exists at $ENV_FILE (not overwriting)"
else
    cat > "$ENV_FILE" << 'ENV_TEMPLATE'
# Production flag
PROD=True

# Port the server listens on (default: 8989)
PORT=8989

# Database file path
DB_PATH=/var/www/weight/weight.db

# Monitor portal log shipping (optional)
MONITOR_URL=
MONITOR_API_KEY=

# Discord webhook URL for new reading notifications (optional)
# DISCORD_WEBHOOK_URL=
ENV_TEMPLATE
    chown www-data:www-data "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    echo "[ok] Created .env template at $ENV_FILE"
fi

# ---------------------------------------------------------------
# 5. Create systemd service
# ---------------------------------------------------------------
SERVICE_FILE="/etc/systemd/system/weight.service"
cat > "$SERVICE_FILE" << 'SERVICE'
[Unit]
Description=Weight Tracker
After=network.target

[Service]
Type=simple
User=www-data
Group=www-data
WorkingDirectory=/var/www/weight
EnvironmentFile=/var/www/weight/.env
ExecStart=/var/www/weight/weight
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ProtectHome=true

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
echo "[ok] Created systemd service at $SERVICE_FILE"

# ---------------------------------------------------------------
# 6. Create the server-side deploy script
# ---------------------------------------------------------------
cat > /usr/local/bin/deploy-weight << 'DEPLOY_SCRIPT'
#!/bin/bash
# /usr/local/bin/deploy-weight
# Runs as root (via sudo) during GitHub Actions deployments.

set -e

DEPLOY_SRC="${1:-/tmp/weight-deploy}"
DEPLOY_DIR=/var/www/weight

# Self-update
BUNDLE_SCRIPT="$DEPLOY_SRC/scripts/deploy-weight"
if [ -f "$BUNDLE_SCRIPT" ] && ! diff -q /usr/local/bin/deploy-weight "$BUNDLE_SCRIPT" > /dev/null 2>&1; then
    echo "[deploy] Updating deploy script from bundle..."
    install -m 755 "$BUNDLE_SCRIPT" /usr/local/bin/deploy-weight
    exec /usr/local/bin/deploy-weight "$@"
fi

SERVICE_USER=$(systemctl show weight --property=User --value)
SERVICE_GROUP=$(systemctl show weight --property=Group --value)

if [ -z "$SERVICE_USER" ]; then
    echo "[deploy] ERROR: Could not read User from weight.service"
    exit 1
fi

echo "[deploy] Stopping service..."
systemctl stop weight || true

echo "[deploy] Installing binary to $DEPLOY_DIR/weight (owner: $SERVICE_USER:$SERVICE_GROUP)..."
rm -f "$DEPLOY_DIR/weight"
cp "$DEPLOY_SRC/weight" "$DEPLOY_DIR/weight"
chmod +x "$DEPLOY_DIR/weight"

echo "[deploy] Updating web assets..."
cp -r "$DEPLOY_SRC/templates/" "$DEPLOY_DIR/"
cp -r "$DEPLOY_SRC/static/"    "$DEPLOY_DIR/"
chown -R "$SERVICE_USER:$SERVICE_GROUP" "$DEPLOY_DIR"

echo "[deploy] Starting service..."
systemctl start weight

echo "[deploy] Verifying service is active..."
sleep 2
if ! systemctl is-active --quiet weight; then
    echo "[deploy] ERROR: Service failed to start. Status:"
    systemctl status weight --no-pager --lines=30
    exit 1
fi

echo "[deploy] Cleaning up..."
rm -rf "$DEPLOY_SRC"

echo "[deploy] Done — weight is running."
DEPLOY_SCRIPT

chmod +x /usr/local/bin/deploy-weight
echo "[ok] Created /usr/local/bin/deploy-weight"

# ---------------------------------------------------------------
# 7. Configure sudoers
# ---------------------------------------------------------------
SUDOERS_FILE="/etc/sudoers.d/weight-deploy"

cat > "$SUDOERS_FILE" << 'EOF'
# Allow the deploy user to run the weight deployment script as root
deploy ALL=(ALL) NOPASSWD: /usr/local/bin/deploy-weight
# Allow stopping the weight service directly (used by the GitHub Actions workflow)
deploy ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop weight
EOF

chmod 440 "$SUDOERS_FILE"
visudo -c -f "$SUDOERS_FILE"
echo "[ok] sudoers entry created at $SUDOERS_FILE"

# ---------------------------------------------------------------
# 8. Configure Caddy reverse proxy
# ---------------------------------------------------------------
CADDY_FILE="/etc/caddy/Caddyfile"
if [ -f "$CADDY_FILE" ]; then
    if grep -q "weight.mchugh.au" "$CADDY_FILE"; then
        echo "[ok] Caddy already has weight.mchugh.au config"
    else
        cat >> "$CADDY_FILE" << 'CADDY'

weight.mchugh.au {
    reverse_proxy localhost:8989
}
CADDY
        systemctl reload caddy
        echo "[ok] Added weight.mchugh.au to Caddy config and reloaded"
    fi
else
    echo "[WARN] Caddy config not found at $CADDY_FILE"
    echo "       Add this to your Caddyfile manually:"
    echo ""
    echo "       weight.mchugh.au {"
    echo "           reverse_proxy localhost:8989"
    echo "       }"
fi

# ---------------------------------------------------------------
# 9. Print next steps
# ---------------------------------------------------------------
echo ""
echo "=== Setup complete ==="
echo ""
echo "If this is a NEW server (no existing deploy key), add these GitHub secrets:"
echo ""
echo "Go to: https://github.com/exploded/weight/settings/secrets/actions"
echo ""
echo "Secret name     : DEPLOY_HOST"
echo "Secret value    : $(hostname -I | awk '{print $1}')"
echo ""
echo "Secret name     : DEPLOY_USER"
echo "Secret value    : $DEPLOY_USER"
echo ""
echo "Secret name     : DEPLOY_SSH_KEY"
echo "Secret value    : (paste the private key below)"
echo ""
echo "---BEGIN PRIVATE KEY (copy everything including the dashes)---"
cat "$KEY_FILE"
echo "---END PRIVATE KEY---"
echo ""
echo "If you already have DEPLOY_HOST/DEPLOY_USER/DEPLOY_SSH_KEY secrets"
echo "from your moon project and this is the same server, you can reuse them."
echo ""
echo "Push to master to trigger your first deployment."
