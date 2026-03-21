FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /mounts-server ./cmd/mounts

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata cifs-utils nfs-utils fuse
COPY --from=builder /mounts-server /usr/local/bin/mounts-server
RUN mkdir -p /packalares/mounts
EXPOSE 8081
ENV MOUNT_BASE_PATH=/packalares/mounts
ENV PORT=8081
ENTRYPOINT ["mounts-server"]
