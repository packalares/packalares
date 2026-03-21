package middleware

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

// RedisProvisioner handles Redis namespace/credential provisioning.
type RedisProvisioner struct {
	host     string
	port     int
	password string
}

func NewRedisProvisioner(host string, port int, password string) *RedisProvisioner {
	return &RedisProvisioner{
		host:     host,
		port:     port,
		password: password,
	}
}

// CreateNamespace creates a Redis namespace (for KVRocks) or validates credentials.
// For standard Redis, this stores the credential mapping in a hash.
func (r *RedisProvisioner) CreateNamespace(ctx context.Context, namespace, token string) error {
	conn, err := r.connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer conn.Close()

	// Store namespace->token mapping for the middleware proxy to use
	key := "packalares:redis:ns:" + namespace
	if err := r.sendCommand(conn, "SET", key, token); err != nil {
		return fmt.Errorf("store redis namespace: %w", err)
	}

	log.Printf("created/updated Redis namespace %q", namespace)
	return nil
}

// DeleteNamespace removes a Redis namespace credential mapping.
func (r *RedisProvisioner) DeleteNamespace(ctx context.Context, namespace string) error {
	conn, err := r.connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer conn.Close()

	key := "packalares:redis:ns:" + namespace
	if err := r.sendCommand(conn, "DEL", key); err != nil {
		return fmt.Errorf("delete redis namespace: %w", err)
	}

	log.Printf("deleted Redis namespace %q", namespace)
	return nil
}

func (r *RedisProvisioner) connect(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	d.Timeout = 5 * time.Second
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", r.host, r.port))
	if err != nil {
		return nil, err
	}

	if r.password != "" {
		if err := r.sendCommand(conn, "AUTH", r.password); err != nil {
			conn.Close()
			return nil, fmt.Errorf("redis AUTH: %w", err)
		}
	}

	return conn, nil
}

func (r *RedisProvisioner) sendCommand(conn net.Conn, args ...string) error {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("*%d\r\n", len(args)))
	for _, arg := range args {
		buf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg))
	}
	if _, err := io.WriteString(conn, buf.String()); err != nil {
		return err
	}
	return r.readResponse(conn)
}

func (r *RedisProvisioner) readResponse(conn net.Conn) error {
	line, err := r.readLine(conn)
	if err != nil {
		return err
	}
	if len(line) == 0 {
		return fmt.Errorf("empty response")
	}
	switch line[0] {
	case '+', ':':
		return nil
	case '-':
		return fmt.Errorf("redis error: %s", line[1:])
	case '$':
		n, _ := strconv.Atoi(line[1:])
		if n > 0 {
			buf := make([]byte, n+2)
			io.ReadFull(conn, buf)
		}
		return nil
	default:
		return nil
	}
}

func (r *RedisProvisioner) readLine(conn net.Conn) (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		_, err := conn.Read(b)
		if err != nil {
			return "", err
		}
		if b[0] == '\n' {
			if len(buf) > 0 && buf[len(buf)-1] == '\r' {
				buf = buf[:len(buf)-1]
			}
			return string(buf), nil
		}
		buf = append(buf, b[0])
	}
}
