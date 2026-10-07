#!/usr/bin/env bash
# openbot uninstaller.
#
#   curl -fsSL https://raw.githubusercontent.com/etchebarne/openbot/main/scripts/uninstall.sh | bash
#
# Removes the openbot service, binary, and config. Your data (/var/lib/openbot: database and
# encryption key) and the openbot system user are kept unless you pass --purge.
#
# Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --purge`):
#   --purge     Also delete all data and the openbot system user (asks for confirmation)
#   --yes       Don't ask for confirmation
#   -h, --help  Show this help
set -euo pipefail

BIN=/usr/local/bin/openbotd
CONF_DIR=/etc/openbot
DATA_DIR=/var/lib/openbot
UNIT=/etc/systemd/system/openbot.service
SERVICE_USER=openbot

bold=$'\e[1m' red=$'\e[31m' green=$'\e[32m' reset=$'\e[0m'
[ -t 1 ] || { bold='' red='' green='' reset=''; }
info() { printf '%s==>%s %s\n' "$bold" "$reset" "$*"; }
ok() { printf '%s✓%s %s\n' "$green" "$reset" "$*"; }
die() { printf '%serror:%s %s\n' "$red" "$reset" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
openbot uninstaller.

  curl -fsSL https://raw.githubusercontent.com/etchebarne/openbot/main/scripts/uninstall.sh | bash

Removes the openbot service, binary, and config. Your data (/var/lib/openbot: database and
encryption key) and the openbot system user are kept unless you pass --purge.

Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --purge`):
  --purge     Also delete all data and the openbot system user (asks for confirmation)
  --yes       Don't ask for confirmation
  -h, --help  Show this help
EOF
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

confirm() {
  local answer
  # Piped runs have no stdin terminal; ask on the tty if there is one.
  if [ -r /dev/tty ] && { : </dev/tty; } 2>/dev/null; then
    printf '%s [y/N] ' "$1" >/dev/tty
    read -r answer </dev/tty || answer=""
  else
    die "can't ask for confirmation without a terminal; rerun with --yes"
  fi
  case "$answer" in y | Y | yes | YES) return 0 ;; *) return 1 ;; esac
}

# Sandbox containers (agents' and the one for local MCP servers) restart with Docker, so they
# always go. Their home volumes are data: they're kept unless purging, and a reinstall reuses them.
remove_sandboxes() {
  local purge=$1 ids volumes images
  command -v docker >/dev/null 2>&1 || return 0
  ids=$(as_root docker ps -aq --filter label=openbot.sandbox 2>/dev/null) || return 0
  if [ -n "$ids" ]; then
    # shellcheck disable=SC2086 # one id per word
    as_root docker rm -f $ids >/dev/null
    ok "Removed openbot's sandbox containers"
  fi
  if $purge; then
    volumes=$(as_root docker volume ls -q --filter name=openbot-sbx- 2>/dev/null) || true
    if [ -n "$volumes" ]; then
      # shellcheck disable=SC2086
      as_root docker volume rm -f $volumes >/dev/null
    fi
    images=$(as_root docker image ls -q openbot-sandbox 2>/dev/null | sort -u) || true
    if [ -n "$images" ]; then
      # shellcheck disable=SC2086
      as_root docker rmi -f $images >/dev/null 2>&1 || true
    fi
    ok "Deleted sandbox files and images"
  fi
}

main() {
  local purge=false yes=false
  while [ $# -gt 0 ]; do
    case "$1" in
      --purge) purge=true; shift ;;
      --yes | -y) yes=true; shift ;;
      -h | --help) usage; exit 0 ;;
      *) die "unknown option: $1 (see --help)" ;;
    esac
  done

  if [ "$(id -u)" -ne 0 ]; then
    command -v sudo >/dev/null || die "run as root or install sudo"
  fi

  if $purge && ! $yes; then
    printf '%sThis permanently deletes all openbot data in %s:%s\n' "$red" "$DATA_DIR" "$reset"
    printf 'your account, agents, chats, memories, sandbox files, and stored credentials.\n'
    confirm "Delete everything?" || die "aborted; nothing was removed"
  fi

  if [ -f "$UNIT" ] || systemctl list-unit-files openbot.service >/dev/null 2>&1; then
    info "Stopping and removing the openbot service"
    as_root systemctl disable --now openbot 2>/dev/null || true
    as_root rm -f "$UNIT"
    as_root systemctl daemon-reload
    as_root systemctl reset-failed openbot 2>/dev/null || true
  fi

  remove_sandboxes "$purge"

  as_root rm -f "$BIN"
  as_root rm -rf "$CONF_DIR"
  ok "Removed the openbot service, binary, and config"

  if $purge; then
    as_root rm -rf "$DATA_DIR"
    if id "$SERVICE_USER" >/dev/null 2>&1; then
      as_root userdel "$SERVICE_USER" 2>/dev/null || as_root deluser "$SERVICE_USER" 2>/dev/null || true
    fi
    ok "Deleted $DATA_DIR and the $SERVICE_USER user"
  elif [ -d "$DATA_DIR" ] || as_root test -d "$DATA_DIR"; then
    echo
    echo "Your data is still in $DATA_DIR. Reinstalling openbot will pick it up again."
    echo "To delete it too, rerun with --purge."
  fi
}

main "$@"
