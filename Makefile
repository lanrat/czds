default: czds

RELEASE_DEPS = fmt lint
include release.mk

DOCKER_REPO := "lanrat/czds"
BUILD_FLAGS := -trimpath -ldflags "-s -w -X main.version=${VERSION}"
SOURCES := $(shell find . -type f -name "*.go") go.mod go.sum

czds: $(SOURCES)
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $@ cmd/*.go

.PHONY: docker
docker: Dockerfile Makefile $(SOURCES)
	docker build --build-arg VERSION=${VERSION} -t $(DOCKER_REPO) .

.PHONY: update-deps
update-deps: deps
	go get -u ./...
	go mod tidy

.PHONY: deps
deps: go.mod
	go mod download
	go mod tidy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: clean
clean:
	rm -rf czds dist/

.PHONY: goreleaser
goreleaser:
	goreleaser release --snapshot --clean
