# syntax=docker/dockerfile:1.7
FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN GOMAXPROCS=1 CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -p=1 -trimpath -ldflags="-s -w" -o /out/handy-parser ./cmd/handy-parser

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && addgroup -g 10001 handy && adduser -D -H -u 10001 -G handy handy
WORKDIR /app
COPY --from=build /out/handy-parser /app/handy-parser
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/handy-parser"]
CMD ["serve"]
