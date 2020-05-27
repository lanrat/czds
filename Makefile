# creates static binaries
CC := CGO_ENABLED=0 go build -ldflags "-w -s" -a -installsuffix cgo

BIN_SOURCES = $(shell find cmd/$(subst bin/,,$@) -maxdepth 1 -type f -name "*.go")
ALL_SOURCES := $(shell find . -type f -name '*.go')
MODULE_SOURCES := $(shell find . -type f -name '*.go' ! -path "./cmd/*" )
CMDS := $(shell ls cmd/)
BINS := $(CMDS:%=bin/%)
CMD_TARGETS = $(@:%=bin/%)

.PHONY: all fmt docker clean install deps $(CMDS) check

all: $(BINS)

.SECONDEXPANSION:
$(BINS): $$(BIN_SOURCES) $(MODULE_SOURCES)
	$(CC) -o $@ $(BIN_SOURCES)

$(CMDS): $$(CMD_TARGETS)

docker: Dockerfile $(SOURCES)
	docker build -t lanrat/czds .

deps: go.mod
	go mod download

fmt:
	gofmt -s -w -l .

check:
	golangci-lint run --exclude-use-default || true
	staticcheck -unused.whole-program -checks all ./...

install: $(SOURCES)
	go install $(CMDS)

clean:
	rm -r $(BINS)
