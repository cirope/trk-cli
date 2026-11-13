# The quality gate (SPEC 8.7). `make check` runs what CI runs, in order.

TOOL      := go tool -modfile=tools/go.mod
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: check fmt tidy vet staticcheck lint test vulncheck cross build

check: fmt tidy vet staticcheck lint test vulncheck cross

fmt:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

tidy:
	go mod tidy -diff
	cd tools && go mod tidy -diff

vet:
	go vet ./...

staticcheck:
	$(TOOL) staticcheck ./...

lint:
	$(TOOL) golangci-lint run

test:
	go test -race ./...

vulncheck:
	$(TOOL) govulncheck ./...

cross:
	@for p in $(PLATFORMS); do \
		echo "go build $$p"; \
		GOOS=$${p%/*} GOARCH=$${p#*/} go build -o /dev/null ./... || exit 1; \
	done

build:
	go build -o bin/trk ./cmd/trk
