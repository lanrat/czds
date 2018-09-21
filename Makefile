
SOURCES := czds-dl.go

.PHONY: all fmt clean install docker

all: czds-dl

czds-dl: $(SOURCES)
	go build -o $@ $(SOURCES)

docker: Dockerfile $(SOURCES)
	docker build -t lanrat/czds-dl .

fmt:
	gofmt -s -w -l .

install: $(SOURCES)
	go install

clean:
	rm -r czds-dl
