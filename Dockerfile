# syntax=docker/dockerfile:1.2
#########################################################
# BUILD IMAGE
#########################################################

FROM golang:1.18 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN go build ./cmd/lortex-proxy

#########################################################
# RELEASE IMAGE
#########################################################

FROM gcr.io/distroless/base:latest AS release
USER nobody
COPY --from=build --chown=root:root /app/lortex-proxy /app/lortex-proxy
ENTRYPOINT ["/app/lortex-proxy"]