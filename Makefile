.PHONY: all backend frontend worker test verify fmt vet clean

all: backend frontend worker

backend:
	cd backend && go build ./...

frontend:
	cd frontend && npm install && npm run build

worker:
	cd worker && go build ./...

test:
	cd backend && go test ./...
	cd worker && go test ./...
	cd frontend && npx tsc --noEmit

verify:
	./scripts/verify.sh

fmt:
	cd backend && go fmt ./...
	cd worker && go fmt ./...

vet:
	cd backend && go vet ./...
	cd worker && go vet ./...

clean:
	rm -rf bin frontend/dist frontend/node_modules