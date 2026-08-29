.PHONY: check test frontend dev build schema

check:
	go fmt ./...
	go vet ./...
	go test ./...
	npm --prefix frontend run lint
	npm --prefix frontend run test
	npm --prefix frontend run build

test:
	go test ./...
	npm --prefix frontend run test

frontend:
	npm --prefix frontend run build

dev:
	cd cmd/aether-desktop && CGO_LDFLAGS='-framework UniformTypeIdentifiers' wails dev -skipembedcreate

build:
	cd cmd/aether-desktop && CGO_LDFLAGS='-framework UniformTypeIdentifiers' wails build -platform darwin/arm64 -clean -skipembedcreate

schema:
	go run ./cmd/schema-export --output schemas/v2
