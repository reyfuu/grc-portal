.PHONY: build run test dev-api dev-web tidy

# Full build: frontend into the Go embed dir, then one static binary.
build:
	cd web && npm install && npm run build
	go build -o bin/grcportal .

run: build
	./bin/grcportal

test:
	go test ./...

tidy:
	go mod tidy

# Dev loop (two terminals): API on :8081 (8080 taken by local Apache), Vite on :5173 proxying /api.
dev-api:
	DEV=1 go run .

dev-web:
	cd web && npm run dev
