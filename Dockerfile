FROM golang:alpine AS builder
ENV GOTOOLCHAIN=auto
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /shiptag ./cmd/shiptag

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git curl bash
COPY --from=builder /shiptag /usr/local/bin/shiptag
ENTRYPOINT ["shiptag"]
