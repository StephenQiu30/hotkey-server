FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
ARG GOPROXY=https://proxy.golang.org,direct
RUN GOPROXY="${GOPROXY}" go mod download && go mod verify

COPY . .
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/hotkey ./cmd/hotkey

FROM alpine:3.23

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S -g 10001 hotkey \
    && adduser -S -D -H -u 10001 -G hotkey hotkey \
    && mkdir -p /var/lib/hotkey/vault \
    && chown -R hotkey:hotkey /var/lib/hotkey

COPY --from=builder --chown=hotkey:hotkey /out/hotkey /usr/local/bin/hotkey

ENV TZ=UTC
WORKDIR /app
USER hotkey

EXPOSE 8080
STOPSIGNAL SIGTERM

ENTRYPOINT ["/usr/local/bin/hotkey"]
CMD ["serve", "--role", "all"]
