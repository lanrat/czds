BIN_SOURCES = $(shell find cmd/$(subst bin/,,$@) -maxdepth 1 -type f -name "*.go")
ALL_SOURCES := $(shell find . -type f -name '*.go')
MODULE_SOURCES := $(shell find . -type f -name '*.go' ! -path "./cmd/*")
CMDS := $(shell ls cmd/)
BINS := $(CMDS:%=bin/%)
CMD_TARGETS = $(@:%=bin/%)
GIT_VERSION = $(shell git describe --tags)
DOCKER_REPO := "lanrat/czds"
DOCKER_IMAGES = $(shell docker image ls --format "{{.Repository}}:{{.Tag}}" $(DOCKER_REPO))

# creates static binaries
CC = CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.version=$(GIT_VERSION)" -installsuffix cgo

.PHONY: all fmt docker clean install deps update-deps $(CMDS) check release

all: $(BINS)

.SECONDEXPANSION:
$(BINS): $$(BIN_SOURCES) $(MODULE_SOURCES)
	$(CC) -o $@ $(BIN_SOURCES)

$(CMDS): $$(CMD_TARGETS)

docker: Dockerfile Makefile $(SOURCES)
	docker build -t $(DOCKER_REPO) -t $(DOCKER_REPO):$(GIT_VERSION) .

update-deps: deps
	GOPROXY=direct go get -u ./...
	go mod tidy

deps: go.mod
	GOPROXY=direct go mod download
	go mod tidy

fmt:
	gofmt -s -w -l .

check:
	golangci-lint run --exclude-use-default || true
	staticcheck -checks all ./...

install: $(SOURCES)
	go install $(CMDS)

clean:
	rm -rf $(BINS)
ifneq ($(shell which docker),)
ifneq ($(DOCKER_IMAGES),)
	docker image rm $(DOCKER_IMAGES)
endif
endif


release: $(BINS)
	$(eval v := $(shell git describe --tags --abbrev=0 | sed -Ee 's/^v|-.*//'))
	@echo current: $v
ifeq ($(bump), major)
	$(eval v := $(shell echo $(v) | awk 'BEGIN{FS=OFS="."} {$$1+=1;$$2=$$3=0} 1'))
else ifeq ($(bump), minor)
	$(eval v := $(shell echo $(v) | awk 'BEGIN{FS=OFS="."} {$$2+=1;$$3=0} 1'))
else
	$(eval v := $(shell echo $(v) | awk 'BEGIN{FS=OFS="."}{$$3+=1;} 1'))
endif
	@echo next: $(v)
	@(git diff --exit-code --shortstat && git diff --exit-code --cached --shortstat)
	git tag v$(v) -m '"release v$(v)"'
	git push && git push --tags

