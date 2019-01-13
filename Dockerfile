FROM golang:alpine

RUN apk update && apk add --no-cache tzdata

WORKDIR /go/app/
COPY . .

ENV CGO_ENABLED=0
RUN go build  -o /go/bin/czds-dl cmd/czds-dl.go

WORKDIR /zones
RUN chown 1000:1000 $(pwd)
USER 1000

ENTRYPOINT [ "czds-dl" ]