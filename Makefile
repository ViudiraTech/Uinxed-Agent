.PHONY: fmt test race vet build check bench
fmt:
	gofmt -w $$(find . -name '*.go')
test:
	go test ./...
race:
	go test -race ./...
vet:
	go vet ./...
build:
	go build ./cmd/ux-agent
check: fmt test race vet build
bench:
	./scripts/benchmark.sh
