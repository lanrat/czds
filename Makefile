
SOURCES := czds-dl.go

.PHONY: all fmt clean install

all: czds-dl

czds-dl: $(SOURCES)
	go build -o $@ $(SOURCES)

fmt:
	gofmt -s -w -l .

install: $(SOURCES)
	go install

clean:
	rm -r czds-dl
