package l4proxy

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"
)

func TestReadClientHello(t *testing.T) {
	// Create a real TLS ClientHello by using crypto/tls to initiate a handshake.
	// We'll capture the ClientHello bytes.
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan struct{})
	var helloBytes []byte
	var sni string
	var readErr error

	go func() {
		defer close(done)
		sni, helloBytes, readErr = readClientHello(serverConn)
	}()

	// Initiate TLS from client side
	go func() {
		tlsConn := tls.Client(clientConn, &tls.Config{
			ServerName:         "test.example.com",
			InsecureSkipVerify: true,
		})
		// The handshake will send ClientHello then hang waiting for ServerHello.
		// That's fine; we just need the ClientHello.
		tlsConn.Handshake() // nolint: will fail, but ClientHello was sent
	}()

	<-done

	if readErr != nil {
		t.Fatalf("readClientHello error: %v", readErr)
	}
	if sni != "test.example.com" {
		t.Errorf("SNI=%q, want test.example.com", sni)
	}
	if len(helloBytes) == 0 {
		t.Error("helloBytes is empty")
	}
}

func TestExtractSNI_NoSNI(t *testing.T) {
	// A minimal valid ClientHello with no SNI extension
	// Record header: ContentType=22 (handshake), Version=TLS1.0, Length
	// Handshake header: Type=1 (ClientHello), Length
	// We build a minimal one manually.
	hello := buildMinimalClientHello("")
	sni := extractSNI(hello)
	if sni != "" {
		t.Errorf("expected empty SNI, got %q", sni)
	}
}

func TestExtractSNI_WithSNI(t *testing.T) {
	hello := buildMinimalClientHello("myhost.example.com")
	sni := extractSNI(hello)
	if sni != "myhost.example.com" {
		t.Errorf("expected myhost.example.com, got %q", sni)
	}
}

// buildMinimalClientHello constructs a minimal TLS ClientHello handshake message
// (without the 5-byte TLS record header) with an optional SNI extension.
func buildMinimalClientHello(serverName string) []byte {
	var buf bytes.Buffer

	// ClientHello body (after handshake type + length)
	var body bytes.Buffer

	// ClientVersion: TLS 1.2 = 0x0303
	body.Write([]byte{0x03, 0x03})

	// Random: 32 bytes
	body.Write(make([]byte, 32))

	// SessionID length: 0
	body.WriteByte(0)

	// CipherSuites: length=2, one cipher
	body.Write([]byte{0x00, 0x02, 0xc0, 0x2f})

	// CompressionMethods: length=1, null
	body.Write([]byte{0x01, 0x00})

	// Extensions
	var exts bytes.Buffer
	if serverName != "" {
		// SNI extension: type=0x0000
		var sniData bytes.Buffer
		nameBytes := []byte(serverName)
		// Server name list length
		sniListLen := 1 + 2 + len(nameBytes) // type(1) + len(2) + name
		sniData.Write([]byte{byte(sniListLen >> 8), byte(sniListLen)})
		sniData.WriteByte(0) // host_name type
		sniData.Write([]byte{byte(len(nameBytes) >> 8), byte(len(nameBytes))})
		sniData.Write(nameBytes)

		exts.Write([]byte{0x00, 0x00}) // extension type: SNI
		exts.Write([]byte{byte(sniData.Len() >> 8), byte(sniData.Len())})
		exts.Write(sniData.Bytes())
	}

	// Extensions total length
	body.Write([]byte{byte(exts.Len() >> 8), byte(exts.Len())})
	body.Write(exts.Bytes())

	// Handshake header: type=1 (ClientHello), length (3 bytes)
	bodyBytes := body.Bytes()
	buf.WriteByte(1) // ClientHello
	buf.Write([]byte{byte(len(bodyBytes) >> 16), byte(len(bodyBytes) >> 8), byte(len(bodyBytes))})
	buf.Write(bodyBytes)

	return buf.Bytes()
}
