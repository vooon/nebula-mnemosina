FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nebula-mnemosina ./cmd/nebula-mnemosina

FROM alpine:3.24

RUN mkdir -p /data && chown 65532:65532 /data

LABEL org.opencontainers.image.description="nebula-mnemosina - Nebula SSH state collector"

COPY --from=build /out/nebula-mnemosina /usr/local/bin/nebula-mnemosina

USER 65532:65532
VOLUME ["/data"]
EXPOSE 12142

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -q -T 2 -O - http://127.0.0.1:12142/healthz >/dev/null || exit 1

ENTRYPOINT ["nebula-mnemosina"]
