# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21
ARG ALPINE_VERSION=3.18

FROM golang:${GO_VERSION} AS build

WORKDIR /usr/src

COPY go.* .
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -extldflags '-static'" \
    -buildvcs=false \
    -tags osusergo,netgo \
    -o /usr/bin/a .

FROM alpine:${ALPINE_VERSION} AS runtime

WORKDIR /opt

# copy binary from build
COPY --from=build /usr/bin/a ./

CMD ["./a"]

EXPOSE 8080