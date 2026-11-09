.PHONY: build frontend test vet run clean

build:
	./scripts/build.sh

frontend:
	./scripts/build-frontend.sh

test:
	go test ./...

vet:
	gofmt -l .
	go vet ./...

run:
	go run ./cmd/task-control serve

clean:
	rm -rf dist internal/web/dist/assets internal/web/dist/index.html
