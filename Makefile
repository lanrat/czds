BIN_SOURCES = $(shell find cmd/$(subst bin/,,$@) -maxdepth 1 -type f -name "*.go")
ALL_SOURCES := $(shell find . -type f -name '*.go')
MODULE_SOURCES := $(shell find . -type f -name '*.go' ! -path "./cmd/*" )
CMDS := $(shell ls cmd/)
BINS := $(CMDS:%=bin/%)
CMD_TARGETS = $(@:%=bin/%)

# creates static binaries
CC := CGO_ENABLED=0 go build -trimpath -ldflags "-w -s -X main.version=$(GIT_VERSION)" -a -installsuffix cgo

.PHONY: all fmt docker clean install deps $(CMDS) check release

all: $(BINS)

.SECONDEXPANSION:
$(BINS): $$(BIN_SOURCES) $(MODULE_SOURCES)
	$(CC) -o $@ $(BIN_SOURCES)

$(CMDS): $$(CMD_TARGETS)

docker: Dockerfile $(SOURCES)
	docker build -t lanrat/czds .

update-deps: deps
	GOPROXY=direct go get -u all
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
