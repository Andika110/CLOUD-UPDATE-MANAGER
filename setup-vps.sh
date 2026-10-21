#!/usr/bin/env bash
# ==============================================================================
# QUINOS CLOUD UPDATE MANAGER — VPS ONE-CLICK SETUP SCRIPT
# Target OS: Ubuntu 22.04 / 24.04 LTS & Debian 11/12
# ==============================================================================

set -e

echo "=================================================="
echo " Starting Quinos Cloud Update Manager Setup"
echo "=================================================="

# 1. Update packages
sudo apt-get update -y
sudo apt-get install -y curl git ufw ca-certificates gnupg lsb-release

# 2. Setup Firewall
echo "[1/4] Configuring UFW Firewall..."
sudo ufw allow 22/tcp || true
sudo ufw allow 80/tcp || true
sudo ufw allow 443/tcp || true
sudo ufw --force enable || true

# 3. Install Docker if not installed
if ! command -v docker &> /dev/null; then
    echo "[2/4] Installing Docker Engine..."
    curl -fsSL https://get.docker.com -o get-docker.sh
    sudo sh get-docker.sh
    rm get-docker.sh
    sudo usermod -aG docker $USER || true
else
    echo "[2/4] Docker is already installed."
fi

# 4. Create directories
echo "[3/4] Preparing directories and permissions..."
mkdir -p data/packages nginx

# 5. Build and Run with Docker Compose
echo "[4/4] Starting Cloud Update Manager containers..."
docker compose down || true
docker compose up -d --build

echo "=================================================="
echo " SETUP COMPLETED SUCCESSFULLY!"
echo " Web Dashboard: http://$(curl -s ifconfig.me):8080/"
echo "=================================================="
