# Dockerfile
FROM golang:1.24 AS builder
WORKDIR /app

# Define build arguments for version, commit, and date.
ARG VERSION=$(git describe --tags --abbrev=0 || echo "0.0.0")
ARG COMMIT=$(git rev-parse --short HEAD || echo "none")
ARG DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

# Copy dependency files and download modules.
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code and compile the gcx binary using ldflags.
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" -v -o ./bin/gcx ./cmd/gcx

# Final image based on Alpine with necessary packages.
FROM golang:1.24-alpine

ENV GOTOOLCHAIN=auto
ENV GOROOT=/usr/local/go

RUN apk --no-cache add ca-certificates git gcc musl-dev mercurial
WORKDIR /app
COPY --from=builder /app/bin/gcx /usr/bin/

CMD ["gcx"]
