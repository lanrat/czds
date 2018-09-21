FROM golang:alpine

WORKDIR /go/src/
COPY . .

RUN go build -o /go/bin/czds-dl

WORKDIR /zones
RUN chown 1000:1000 $(pwd)
USER 1000

ENTRYPOINT [ "czds-dl" ]