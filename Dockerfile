FROM golang:1.26-alpine AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /shiptag ./cmd/shiptag

# Runs as root on purpose: GitHub Docker actions require it to access the
# workspace, and CI job images write outputs into root-owned clone directories.
FROM alpine:3.22
RUN apk add --no-cache git ca-certificates \
    && git config --system --add safe.directory '*'
COPY --from=builder /shiptag /usr/local/bin/shiptag
ENTRYPOINT ["shiptag"]
