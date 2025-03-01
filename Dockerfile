# Dockerfile
FROM golang:1.24 AS builder
WORKDIR /app

# Define build arguments for version, commit, and date.
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

# Copy dependency files and download modules.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and compile the gcx binary using ldflags.
COPY . .
RUN go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -o gcx ./cmd/gcx

# Final image based on Alpine with necessary packages.
FROM alpine:latest
RUN apk --no-cache add ca-certificates git
WORKDIR /root/
COPY --from=builder /app/gcx .

ENTRYPOINT ["/root/gcx"]
