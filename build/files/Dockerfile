FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /files-server ./cmd/files

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /files-server /usr/local/bin/files-server
RUN mkdir -p /packalares/data
EXPOSE 8080
ENV DATA_PATH=/packalares/data
ENV PORT=8080
ENTRYPOINT ["files-server"]
LABEL org.opencontainers.image.description="File server — WebDAV-compatible file manager with upload, download, rename, permissions, and symlink protection"
