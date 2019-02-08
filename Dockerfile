# build stage
FROM golang:alpine AS czds-build-env
RUN apk update && apk add --no-cache make

WORKDIR /go/app/
COPY . .
RUN make


# final stage
FROM alpine
RUN apk update && apk add --no-cache tzdata ca-certificates
COPY --from=czds-build-env /go/app/bin/* /usr/local/bin/

WORKDIR /zones
RUN chown 1000:1000 $(pwd)
USER 1000

CMD [ "czds-dl" ]