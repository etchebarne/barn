#!/usr/bin/env bash
# barn installer for Linux servers (systemd).
#
#   curl -fsSL https://raw.githubusercontent.com/etchebarne/barn/main/scripts/install.sh | bash
#
# Installs (or upgrades) barnd as a systemd service:
#   /usr/local/bin/barnd          the server (web app included)
#   /etc/barn/barn.env            configuration (kept on upgrade)
#   /var/lib/barn                 data: database and encryption key (kept on upgrade)
#
# Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --tailscale`):
#   --version <tag>   Install a specific release (default: latest)
#   --addr <host:port>
#                     Address to listen on (default: Tailscale IP if available, else 127.0.0.1:8080)
#   --tailscale       Install Tailscale if missing, connect, and listen on the tailnet
#   --no-docker       Don't install Docker (agents then have no sandbox to run commands in)
#   -h, --help        Show this help
set -euo pipefail

REPO="etchebarne/barn"
PORT=8080
BIN=/usr/local/bin/barnd
CONF_DIR=/etc/barn
CONF="$CONF_DIR/barn.env"
DATA_DIR=/var/lib/barn
UNIT=/etc/systemd/system/barn.service
SERVICE_USER=barn

TMP_DIRS=()
cleanup() { [ ${#TMP_DIRS[@]} -eq 0 ] || rm -rf "${TMP_DIRS[@]}"; }
trap cleanup EXIT

bold=$'\e[1m' dim=$'\e[2m' red=$'\e[31m' green=$'\e[32m' reset=$'\e[0m'
[ -t 1 ] || { bold='' dim='' red='' green='' reset=''; }
info() { printf '%s==>%s %s\n' "$bold" "$reset" "$*"; }
ok() { printf '%s✓%s %s\n' "$green" "$reset" "$*"; }
die() { printf '%serror:%s %s\n' "$red" "$reset" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
barn installer for Linux servers (systemd).

  curl -fsSL https://raw.githubusercontent.com/etchebarne/barn/main/scripts/install.sh | bash

Installs (or upgrades) barnd as a systemd service:
  /usr/local/bin/barnd          the server (web app included)
  /etc/barn/barn.env            configuration (kept on upgrade)
  /var/lib/barn                 data: database and encryption key (kept on upgrade)

Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --tailscale`):
  --version <tag>   Install a specific release (default: latest)
  --addr <host:port>
                    Address to listen on (default: Tailscale IP if available, else 127.0.0.1:8080)
  --tailscale       Install Tailscale if missing, connect, and listen on the tailnet
  -h, --help        Show this help
EOF
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

need_sudo() {
  [ "$(id -u)" -eq 0 ] && return
  command -v sudo >/dev/null || die "run as root or install sudo"
  sudo -n true 2>/dev/null && return
  # Piped installs have no stdin terminal; read the sudo password from the tty instead.
  if [ -r /dev/tty ] && { : </dev/tty; } 2>/dev/null; then
    # shellcheck disable=SC2024 # the tty is for sudo's password prompt
    sudo -v </dev/tty || die "sudo is required"
  else
    sudo -v || die "sudo is required"
  fi
}

pkg_install() {
  if command -v apt-get >/dev/null; then
    as_root env DEBIAN_FRONTEND=noninteractive apt-get update -qq
    as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" >/dev/null
  elif command -v dnf >/dev/null; then
    as_root dnf install -y -q "$@"
  elif command -v yum >/dev/null; then
    as_root yum install -y -q "$@"
  elif command -v zypper >/dev/null; then
    as_root zypper --non-interactive install "$@"
  elif command -v pacman >/dev/null; then
    as_root pacman -Sy --noconfirm --needed "$@"
  elif command -v apk >/dev/null; then
    as_root apk add --no-cache "$@"
  else
    die "couldn't find a package manager to install: $*"
  fi
}

ensure_prerequisites() {
  local missing=()
  command -v curl >/dev/null || missing+=(curl)
  command -v tar >/dev/null || missing+=(tar)
  command -v sha256sum >/dev/null || missing+=(coreutils)
  [ -e /etc/ssl/certs/ca-certificates.crt ] || [ -e /etc/pki/tls/certs/ca-bundle.crt ] || missing+=(ca-certificates)
  if [ ${#missing[@]} -gt 0 ]; then
    info "Installing prerequisites: ${missing[*]}"
    pkg_install "${missing[@]}"
  fi
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) echo amd64 ;;
    aarch64 | arm64) echo arm64 ;;
    *) die "unsupported architecture $(uname -m) (barn supports amd64 and arm64)" ;;
  esac
}

latest_version() {
  local url
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest") ||
    die "couldn't reach GitHub to find the latest release"
  local tag="${url##*/}"
  if [ -z "$tag" ] || [ "$tag" = latest ]; then
    die "no releases published yet for $REPO"
  fi
  echo "$tag"
}

tailscale_ip() {
  command -v tailscale >/dev/null || return 1
  tailscale ip -4 2>/dev/null | head -n1 | grep -E '^[0-9.]+$'
}

setup_tailscale() {
  if ! command -v tailscale >/dev/null; then
    info "Installing Tailscale"
    curl -fsSL https://tailscale.com/install.sh | sh
  fi
  if ! tailscale_ip >/dev/null; then
    info "Connecting to your tailnet (follow the link below to sign in)"
    as_root tailscale up </dev/tty
  fi
  tailscale_ip >/dev/null || die "Tailscale is installed but not connected"
}

install_binary() {
  local version="$1" arch="$2" tmp
  tmp=$(mktemp -d)
  TMP_DIRS+=("$tmp")
  local name="barn_${version}_linux_${arch}"
  local base="${BARN_DOWNLOAD_URL:-https://github.com/$REPO/releases/download/$version}"
  info "Downloading barn $version ($arch)"
  curl -fsSL -o "$tmp/$name.tar.gz" "$base/$name.tar.gz" || die "download failed: $base/$name.tar.gz"
  curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || die "couldn't download checksums"
  (cd "$tmp" && grep " $name.tar.gz\$" checksums.txt | sha256sum -c --quiet -) ||
    die "checksum verification failed"
  tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
  as_root install -m 0755 "$tmp/$name/barnd" "$BIN"
  ok "Installed $BIN ($("$BIN" --version))"
}

ensure_user() {
  id "$SERVICE_USER" >/dev/null 2>&1 && return
  info "Creating system user $SERVICE_USER"
  local nologin
  nologin=$(command -v nologin || echo /bin/false)
  if command -v useradd >/dev/null; then
    as_root useradd --system --home-dir "$DATA_DIR" --no-create-home --shell "$nologin" "$SERVICE_USER"
  else
    as_root adduser -S -D -H -h "$DATA_DIR" -s "$nologin" "$SERVICE_USER"
  fi
}

write_config() {
  local addr="$1"
  as_root install -d -m 0750 -o root -g "$SERVICE_USER" "$CONF_DIR"
  as_root install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_USER" "$DATA_DIR"
  if as_root test -f "$CONF"; then
    if [ -n "$addr" ]; then
      as_root sed -i "s|^BARN_ADDR=.*|BARN_ADDR=$addr|" "$CONF"
      ok "Updated BARN_ADDR in $CONF"
    else
      ok "Keeping existing config $CONF"
    fi
    return
  fi
  [ -n "$addr" ] || addr="127.0.0.1:$PORT"
  as_root tee "$CONF" >/dev/null <<EOF
# barn server settings. Restart after editing: sudo systemctl restart barn
# Everything else (model provider, agents) is configured in the app.

# Address to listen on. Use your Tailscale IP to keep barn private to your tailnet.
BARN_ADDR=$addr

# Where barn stores its database and encryption key.
BARN_DATA_DIR=$DATA_DIR

# Set to true if you serve barn over HTTPS (e.g. behind a reverse proxy).
BARN_SECURE_COOKIES=false

# Where connected apps (GitHub, Linear, Render, webhooks) can reach barn to deliver events, e.g. a
# Tailscale Funnel URL for /hooks/* only. Slack and MCP don't need it.
# BARN_PUBLIC_URL=https://your-funnel-name.ts.net
EOF
  as_root chmod 0640 "$CONF"
  as_root chown root:"$SERVICE_USER" "$CONF"
  ok "Wrote $CONF"
}

# Agents run commands in Docker sandboxes. barn talks to the Docker daemon, so the barn user
# joins the docker group (which is root-equivalent on the host).
setup_docker() {
  if ! command -v docker >/dev/null; then
    info "Installing Docker (agents' sandboxes); this takes a minute"
    local log
    log=$(mktemp)
    # Docker's script is chatty (apt, needrestart); show its output only if it fails.
    if ! curl -fsSL https://get.docker.com | as_root env NEEDRESTART_MODE=a sh >"$log" 2>&1; then
      tail -n 20 "$log" >&2
      rm -f "$log"
      die "installing Docker failed (rerun with --no-docker to skip sandboxes)"
    fi
    rm -f "$log"
  fi
  as_root systemctl enable --now --quiet docker 2>/dev/null || true
  if ! id -nG "$SERVICE_USER" | tr ' ' '\n' | grep -qx docker; then
    as_root usermod -aG docker "$SERVICE_USER"
  fi
  ok "Docker is ready for sandboxes"
}

write_unit() {
  as_root tee "$UNIT" >/dev/null <<EOF
[Unit]
Description=barn - persistent AI agents
Documentation=https://github.com/$REPO
Wants=network-online.target
After=network-online.target tailscaled.service docker.service

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
SupplementaryGroups=$(id -nG "$SERVICE_USER" | tr ' ' '\n' | grep -qx docker && echo docker)
EnvironmentFile=$CONF
ExecStart=$BIN
Restart=on-failure
RestartSec=5

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
PrivateDevices=true
ReadWritePaths=$DATA_DIR
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
}

wait_healthy() {
  local addr="$1"
  for _ in $(seq 1 30); do
    if curl -fs -o /dev/null "http://$addr/api/auth/status"; then return 0; fi
    sleep 0.5
  done
  return 1
}

main() {
  local version="" addr="" use_tailscale=false use_docker=true
  while [ $# -gt 0 ]; do
    case "$1" in
      --version) version="${2:?--version needs a value}"; shift 2 ;;
      --addr) addr="${2:?--addr needs a value}"; shift 2 ;;
      --tailscale) use_tailscale=true; shift ;;
      --no-docker) use_docker=false; shift ;;
      -h | --help) usage; exit 0 ;;
      *) die "unknown option: $1 (see --help)" ;;
    esac
  done

  [ "$(uname -s)" = Linux ] || die "barn's installer supports Linux only"
  if ! command -v systemctl >/dev/null || [ ! -d /run/systemd/system ]; then
    die "systemd is required"
  fi
  local arch
  arch=$(detect_arch)

  need_sudo
  ensure_prerequisites

  if $use_tailscale; then
    setup_tailscale
  fi
  if [ -z "$addr" ] && { $use_tailscale || ! as_root test -f "$CONF"; }; then
    if ip=$(tailscale_ip); then addr="$ip:$PORT"; fi
  fi

  [ -n "$version" ] || version=$(latest_version)
  local upgrading=false
  [ -x "$BIN" ] && upgrading=true

  install_binary "$version" "$arch"
  ensure_user
  if $use_docker; then
    setup_docker
  fi
  write_config "$addr"
  write_unit
  as_root systemctl daemon-reload
  as_root systemctl enable --quiet barn
  if $upgrading; then
    as_root systemctl restart barn
  else
    as_root systemctl start barn
  fi

  local listen
  listen=$(as_root sed -n 's/^BARN_ADDR=//p' "$CONF")
  if wait_healthy "$listen"; then
    ok "barn is running on $listen"
  else
    die "barn didn't start; check: sudo journalctl -u barn -n 50"
  fi

  echo
  local host="${listen%:*}" port="${listen##*:}"
  case "$host" in
    127.0.0.1 | localhost)
      printf '%sOpen barn%s through an SSH tunnel from your computer:\n\n' "$bold" "$reset"
      printf '  ssh -L %s:127.0.0.1:%s %s@%s\n\n' "$port" "$port" "$(id -un)" "$(hostname -f 2>/dev/null || hostname)"
      printf 'then visit http://localhost:%s\n\n' "$port"
      printf '%sTip: rerun with --tailscale to reach barn from all your devices.%s\n' "$dim" "$reset"
      ;;
    *)
      printf '%sOpen barn:%s http://%s\n' "$bold" "$reset" "$listen"
      ;;
  esac
  echo
  printf '%sManage:%s sudo systemctl {status,restart,stop} barn · logs: sudo journalctl -u barn -f\n' "$dim" "$reset"
  printf '%sUpgrade:%s rerun this installer · %sUninstall:%s curl -fsSL https://raw.githubusercontent.com/%s/main/scripts/uninstall.sh | bash\n' \
    "$dim" "$reset" "$dim" "$reset" "$REPO"
}

main "$@"
