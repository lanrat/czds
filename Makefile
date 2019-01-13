
SOURCES := czds.go jwt/jwt.go cmd/czds-dl.go

.PHONY: all fmt clean install docker

all: czds-dl

czds-dl: $(SOURCES)
	go build -o $@ cmd/czds-dl.go

docker: Dockerfile $(SOURCES)
	docker build -t lanrat/czds .

fmt:
	gofmt -s -w -l .

install: $(SOURCES)
	go install

clean:
	rm -r czds-dl
