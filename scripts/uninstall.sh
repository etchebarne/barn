#!/usr/bin/env bash
# barn uninstaller.
#
#   curl -fsSL https://raw.githubusercontent.com/etchebarne/barn/main/scripts/uninstall.sh | bash
#
# Removes the barn service, binary, and config. Your data (/var/lib/barn: database and
# encryption key) and the barn system user are kept unless you pass --purge.
#
# Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --purge`):
#   --purge     Also delete all data and the barn system user (asks for confirmation)
#   --yes       Don't ask for confirmation
#   -h, --help  Show this help
set -euo pipefail

BIN=/usr/local/bin/barnd
CONF_DIR=/etc/barn
DATA_DIR=/var/lib/barn
UNIT=/etc/systemd/system/barn.service
SERVICE_USER=barn

bold=$'\e[1m' red=$'\e[31m' green=$'\e[32m' reset=$'\e[0m'
[ -t 1 ] || { bold='' red='' green='' reset=''; }
info() { printf '%s==>%s %s\n' "$bold" "$reset" "$*"; }
ok() { printf '%s✓%s %s\n' "$green" "$reset" "$*"; }
die() { printf '%serror:%s %s\n' "$red" "$reset" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
barn uninstaller.

  curl -fsSL https://raw.githubusercontent.com/etchebarne/barn/main/scripts/uninstall.sh | bash

Removes the barn service, binary, and config. Your data (/var/lib/barn: database and
encryption key) and the barn system user are kept unless you pass --purge.

Options (pass after `bash -s --` when piping, e.g. `| bash -s -- --purge`):
  --purge     Also delete all data and the barn system user (asks for confirmation)
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
    printf '%sThis permanently deletes all barn data in %s:%s\n' "$red" "$DATA_DIR" "$reset"
    printf 'your account, agents, chats, memories, and stored credentials.\n'
    confirm "Delete everything?" || die "aborted; nothing was removed"
  fi

  if [ -f "$UNIT" ] || systemctl list-unit-files barn.service >/dev/null 2>&1; then
    info "Stopping and removing the barn service"
    as_root systemctl disable --now barn 2>/dev/null || true
    as_root rm -f "$UNIT"
    as_root systemctl daemon-reload
    as_root systemctl reset-failed barn 2>/dev/null || true
  fi

  as_root rm -f "$BIN"
  as_root rm -rf "$CONF_DIR"
  ok "Removed the barn service, binary, and config"

  if $purge; then
    as_root rm -rf "$DATA_DIR"
    if id "$SERVICE_USER" >/dev/null 2>&1; then
      as_root userdel "$SERVICE_USER" 2>/dev/null || as_root deluser "$SERVICE_USER" 2>/dev/null || true
    fi
    ok "Deleted $DATA_DIR and the $SERVICE_USER user"
  elif [ -d "$DATA_DIR" ] || as_root test -d "$DATA_DIR"; then
    echo
    echo "Your data is still in $DATA_DIR. Reinstalling barn will pick it up again."
    echo "To delete it too, rerun with --purge."
  fi
}

main "$@"
