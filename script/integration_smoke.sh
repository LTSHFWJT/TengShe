#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${TENGSHE_BIN_DIR:-/tmp/tengshe-it}"
GOCACHE_DIR="${GOCACHE:-/tmp/tengshe-gocache}"
SECRET="${TENGSHE_SECRET:-test}"
ADMIN_PORT="${TENGSHE_ADMIN_PORT:-9999}"
AGENT1_PORT="${TENGSHE_AGENT1_PORT:-10001}"
SOCKS_PORT="${TENGSHE_SOCKS_PORT:-1080}"
FORWARD_PORT="${TENGSHE_FORWARD_PORT:-18080}"
BACKWARD_LPORT="${TENGSHE_BACKWARD_LPORT:-18081}"
BACKWARD_RPORT="${TENGSHE_BACKWARD_RPORT:-18082}"
WS_PORT="${TENGSHE_WS_PORT:-18080}"
WS_CHILD_PORT="${TENGSHE_WS_CHILD_PORT:-18081}"
WS_PATH="${TENGSHE_WS_PATH:-/tengshe}"

ADMIN_BIN="${BIN_DIR}/tengshe_admin"
AGENT_BIN="${BIN_DIR}/tengshe_agent"

build() {
	mkdir -p "${BIN_DIR}"
	(
		cd "${ROOT_DIR}"
		GOCACHE="${GOCACHE_DIR}" go build -o "${ADMIN_BIN}" ./admin
		GOCACHE="${GOCACHE_DIR}" go build -o "${AGENT_BIN}" ./agent
	)
}

single() {
	build
	cat <<EOF
# Terminal A
${ADMIN_BIN} -l ${ADMIN_PORT} -s ${SECRET}

# Terminal B
${AGENT_BIN} -c 127.0.0.1:${ADMIN_PORT} -s ${SECRET}

# Admin commands
topo
goto 0
detail
status
EOF
}

multihop() {
	build
	cat <<EOF
# Terminal A
${ADMIN_BIN} -l ${ADMIN_PORT} -s ${SECRET}

# Terminal B
${AGENT_BIN} -c 127.0.0.1:${ADMIN_PORT} -s ${SECRET}

# Admin commands after agent1 appears
topo
goto 0
listen ${AGENT1_PORT}

# Terminal C
${AGENT_BIN} -c 127.0.0.1:${AGENT1_PORT} -s ${SECRET}

# Admin commands
topo
goto 1
detail
status
EOF
}

ws_single() {
	build
	cat <<EOF
# Terminal A
${ADMIN_BIN} -p ws -l 127.0.0.1:${WS_PORT}${WS_PATH} -s ${SECRET}

# Terminal B
${AGENT_BIN} -p ws -c ws://127.0.0.1:${WS_PORT}${WS_PATH} -s ${SECRET}

# Admin commands
topo
goto 0
detail
status
EOF
}

ws_multihop() {
	build
	cat <<EOF
# Terminal A
${ADMIN_BIN} -p ws -l 127.0.0.1:${WS_PORT}${WS_PATH} -s ${SECRET}

# Terminal B
${AGENT_BIN} -p ws -c ws://127.0.0.1:${WS_PORT}${WS_PATH} -s ${SECRET}

# Admin commands after agent1 appears
topo
goto 0
listen
# Choose: 1. Normal passive, 4. WS
127.0.0.1:${WS_CHILD_PORT}${WS_PATH}

# Terminal C
${AGENT_BIN} -p ws -c ws://127.0.0.1:${WS_CHILD_PORT}${WS_PATH} -s ${SECRET}

# Admin commands
topo
goto 1
detail
status
EOF
}

socks() {
	cat <<EOF
# Run after single-link setup and 'goto 0'
socks 127.0.0.1:${SOCKS_PORT}
status
# Optional local check from another terminal:
# curl --socks5 127.0.0.1:${SOCKS_PORT} http://127.0.0.1:${FORWARD_PORT}
stopsocks
status
EOF
}

forward_backward() {
	cat <<EOF
# Prepare a local service for forward testing in another terminal:
# python3 -m http.server ${FORWARD_PORT}

# Run after single-link setup and 'goto 0'
forward ${FORWARD_PORT} 127.0.0.1:${FORWARD_PORT}
status
stopforward

backward ${BACKWARD_LPORT} ${BACKWARD_RPORT}
status
stopbackward
EOF
}

file_transfer() {
	cat <<EOF
# Prepare a small file on the side that will upload it:
# printf 'tengshe-smoke\\n' > /tmp/tengshe-small.txt

# Run after single-link setup and 'goto 0'
upload /tmp/tengshe-small.txt
download /tmp/tengshe-small.txt
EOF
}

usage() {
	cat <<EOF
Usage: $0 <command>

Commands:
  build             Build admin and agent into ${BIN_DIR}
  single            Print admin listen + agent connect smoke commands
  multihop          Print admin -> agent1 -> agent2 smoke commands
  ws-single         Print WS admin listen + agent connect smoke commands
  ws-multihop       Print WS admin -> agent1 -> agent2 smoke commands
  socks             Print SOCKS5 TCP smoke commands
  forward-backward  Print forward/backward smoke commands
  file              Print upload/download small-file smoke commands
  all               Print all smoke flows

Environment:
  TENGSHE_BIN_DIR, GOCACHE, TENGSHE_SECRET, TENGSHE_ADMIN_PORT,
  TENGSHE_AGENT1_PORT, TENGSHE_SOCKS_PORT, TENGSHE_FORWARD_PORT,
  TENGSHE_BACKWARD_LPORT, TENGSHE_BACKWARD_RPORT,
  TENGSHE_WS_PORT, TENGSHE_WS_CHILD_PORT, TENGSHE_WS_PATH
EOF
}

case "${1:-help}" in
build)
	build
	;;
single)
	single
	;;
multihop)
	multihop
	;;
ws-single)
	ws_single
	;;
ws-multihop)
	ws_multihop
	;;
socks)
	socks
	;;
forward-backward)
	forward_backward
	;;
file)
	file_transfer
	;;
all)
	single
	printf '\n'
	multihop
	printf '\n'
	ws_single
	printf '\n'
	ws_multihop
	printf '\n'
	socks
	printf '\n'
	forward_backward
	printf '\n'
	file_transfer
	;;
help|-h|--help)
	usage
	;;
*)
	usage
	exit 2
	;;
esac
