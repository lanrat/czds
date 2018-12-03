FROM golang:alpine

RUN apk update && apk add --no-cache git tzdata

WORKDIR /go/app/
COPY . .

ENV CGO_ENABLED=0
#RUN go get .
RUN go build -o /go/bin/czds-dl

WORKDIR /zones
RUN chown 1000:1000 $(pwd)
USER 1000

ENTRYPOINT [ "czds-dl" ]
