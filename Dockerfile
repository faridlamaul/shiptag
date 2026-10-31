# Base images are pinned by digest; Dependabot keeps tag and digest current.
FROM golang:1.27-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /shiptag ./cmd/shiptag

# Runs as root on purpose: GitHub Docker actions require it to access the
# workspace, and CI job images write outputs into root-owned clone directories.
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk add --no-cache git ca-certificates \
    && git config --system --add safe.directory '*'
COPY --from=builder /shiptag /usr/local/bin/shiptag
ENTRYPOINT ["shiptag"]
