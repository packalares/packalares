FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
COPY index.html ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /file-manager .

FROM alpine:3.20

RUN apk add --no-cache \
    ca-certificates \
    cifs-utils \
    nfs-utils \
    rclone \
    fuse3 \
    && mkdir -p /packalares/data

COPY --from=builder /file-manager /usr/local/bin/file-manager

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/file-manager"]
