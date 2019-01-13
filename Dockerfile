FROM golang:alpine

WORKDIR /app/
COPY . .

RUN CGO_ENABLED=0 go build  -o /go/bin/czds-dl cmd/czds-dl.go

WORKDIR /zones
RUN chown 1000:1000 $(pwd)
USER 1000

ENTRYPOINT [ "czds-dl" ]