#!/usr/bin/env bash
# Manage the DRISHTI HyperShift stack on Podman (no compose needed).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

POD="hypershift"
BACK="hs-backend"
FRONT="hs-frontend"
WORKER="hs-worker"
BACK_TAG="localhost/hypershift-backend:0.1"
FRONT_TAG="localhost/hypershift-frontend:0.1"
WORKER_TAG="localhost/hypershift-worker:0.1"

build() {
  echo "Building images..."
  podman build -t "$BACK_TAG"   ./backend
  podman build -t "$FRONT_TAG"  ./frontend
  podman build -t "$WORKER_TAG" ./worker
}

down() {
  echo "Stopping pod '$POD'..."
  podman pod stop "$POD" 2>/dev/null || true
  podman pod rm   "$POD" --force 2>/dev/null || true
  for c in "$BACK" "$FRONT" "$WORKER"; do podman rm "$c" --force 2>/dev/null || true; done
}

up() {
  down
  echo "Creating pod '$POD'..."
  podman pod create --name "$POD" -p 8080:8080 -p 5173:80 -p 8090:8090 2>/dev/null || true

  echo "Starting backend..."
  podman run -d --pod "$POD" --name "$BACK" \
    -e DRISHTI_MODE=mock -e DRISHTI_HTTP_ADDR=:8080 -e DRISHTI_LOG_LEVEL=info \
    "$BACK_TAG" >/dev/null

  echo "Starting worker..."
  podman run -d --pod "$POD" --name "$WORKER" \
    -e DRISHTI_WORKER_ADDR=:8090 -e DRISHTI_LOG_LEVEL=info \
    "$WORKER_TAG" >/dev/null

  echo "Starting frontend..."
  podman run -d --pod "$POD" --name "$FRONT" "$FRONT_TAG" >/dev/null

  echo ""
  echo "Stack is up:"
  echo "  Frontend:  http://localhost:5173"
  echo "  Backend:   http://localhost:8080"
  echo "  Worker:    http://localhost:8090"
}

status_() {
  podman pod ps --filter "name=$POD"
  echo ""
  podman ps -a --filter "pod=$POD" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
}

logs_() {
  podman logs "$BACK"
  echo "----- worker -----"
  podman logs "$WORKER"
  echo "----- frontend -----"
  podman logs "$FRONT"
}

test_() {
  echo "Waiting for containers to start..."
  sleep 3
  echo; echo "[backend] GET /healthz"
  curl -sf "http://localhost:8080/healthz" && echo
  echo; echo "[backend] GET /api/v1/connections"
  curl -sf "http://localhost:8080/api/v1/connections" && echo
  echo; echo "[worker] GET /healthz"
  curl -sf "http://localhost:8090/healthz" && echo
  echo; echo "[frontend] GET / (status only)"
  curl -s -o /dev/null -w "HTTP %{http_code}\n" "http://localhost:5173/"
  echo; echo "[backend] POST /api/v1/plans (DRAFT only)"
  curl -sf -X POST "http://localhost:8080/api/v1/plans" \
    -H "Content-Type: application/json" \
    -d '{"source_vm_id":"vm-web-01","source_connection_id":"conn-vmware-lab","target_node_id":"node-pve-01","target_connection_id":"conn-proxmox-lab"}' && echo
}

case "${1:-up}" in
  build)   build ;;
  up)      up ;;
  down)    down ;;
  status)  status_ ;;
  logs)    logs_ ;;
  test)    test_ ;;
  restart) down; up ;;
  *) echo "Usage: $0 {build|up|down|status|logs|test|restart}"; exit 1 ;;
esac