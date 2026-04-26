FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nebula-mnemosina ./cmd/nebula-mnemosina

FROM scratch

COPY --from=build /out/nebula-mnemosina /nebula-mnemosina
COPY --chown=65532:65532 docker/rootfs/data /data

USER 65532:65532
VOLUME ["/data"]
EXPOSE 12142

ENTRYPOINT ["/nebula-mnemosina"]
