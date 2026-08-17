#!/bin/sh
# Lightweight POSIX watchdog for environments without systemd.
# Restarts `molex serve` when the process exits, and optionally probes /healthz.
#
# Usage:
#   MOLEX_BIN=/usr/local/bin/molex \
#   MOLEX_CONFIG=/var/lib/molex/molex.json \
#   MOLEX_HEALTH=http://127.0.0.1:8080/healthz \
#   ./deploy/molex-keepalive.sh

set -eu

MOLEX_BIN="${MOLEX_BIN:-molex}"
MOLEX_CONFIG="${MOLEX_CONFIG:-./molex.json}"
MOLEX_HEALTH="${MOLEX_HEALTH:-}"
MOLEX_RESTART_SEC="${MOLEX_RESTART_SEC:-2}"
MOLEX_HEALTH_SEC="${MOLEX_HEALTH_SEC:-20}"

log() {
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2
}

health_ok() {
  [ -z "$MOLEX_HEALTH" ] && return 0
  if command -v curl >/dev/null 2>&1; then
    curl --fail --silent --max-time 3 "$MOLEX_HEALTH" >/dev/null
    return $?
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -q -O /dev/null --timeout=3 "$MOLEX_HEALTH"
    return $?
  fi
  return 0
}

trap 'if [ -n "${child:-}" ]; then kill "$child" 2>/dev/null || true; wait "$child" 2>/dev/null || true; fi; exit 0' INT TERM

while :; do
  log "starting $MOLEX_BIN serve --config $MOLEX_CONFIG"
  "$MOLEX_BIN" serve --config "$MOLEX_CONFIG" &
  child=$!

  while kill -0 "$child" 2>/dev/null; do
    if ! health_ok; then
      log "health check failed; restarting"
      kill "$child" 2>/dev/null || true
      wait "$child" 2>/dev/null || true
      break
    fi
    sleep "$MOLEX_HEALTH_SEC"
  done

  wait "$child" 2>/dev/null || true
  log "process exited; retrying in ${MOLEX_RESTART_SEC}s"
  sleep "$MOLEX_RESTART_SEC"
done
