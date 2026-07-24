#!/usr/bin/env bash
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

pass=0
fail=0
step() {
  echo ""
  echo "=== $1 ==="
}
check() {
  if [ $? -eq 0 ]; then
    echo "OK"
    pass=$((pass+1))
  else
    echo "FAILED"
    fail=$((fail+1))
  fi
}

step "Backend build";      (cd backend && go build ./...);                 check
step "Backend go vet";     (cd backend && go vet ./...);                   check
step "Backend tests";      (cd backend && go test ./...);                  check
step "Worker build";       (cd worker  && go build ./...);                 check
step "Worker go vet";      (cd worker  && go vet ./...);                   check
step "Worker tests";       (cd worker  && go test ./...);                  check
step "Frontend typecheck"; (cd frontend && npx tsc --noEmit);              check
step "Frontend build";     (cd frontend && npm run build);                 check

echo ""
echo "==================== SUMMARY ===================="
echo "PASS: $pass  FAIL: $fail"
[ "$fail" -eq 0 ]