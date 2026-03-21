package l4proxy

import (
	"fmt"
	"io"
)

// readClientHello reads a TLS ClientHello from the connection and extracts
// the SNI server name. Returns empty string if no SNI extension is present
// (e.g., raw IP access). Returns the raw ClientHello bytes so they can be
// replayed to the upstream.
func readClientHello(conn io.Reader) (serverName string, helloBytes []byte, err error) {
	// TLS record header: 5 bytes
	header := make([]byte, 5)
	n, err := io.ReadFull(conn, header)
	if err != nil {
		return "", nil, fmt.Errorf("read TLS header: %w", err)
	}
	helloBytes = append(helloBytes, header[:n]...)

	// ContentType must be Handshake (22)
	if header[0] != 22 {
		return "", helloBytes, nil
	}

	// Record length
	recordLen := int(header[3])<<8 | int(header[4])
	if recordLen > 16384 {
		return "", helloBytes, fmt.Errorf("TLS record too large: %d", recordLen)
	}

	payload := make([]byte, recordLen)
	n, err = io.ReadFull(conn, payload)
	if err != nil {
		return "", append(helloBytes, payload[:n]...), fmt.Errorf("read TLS payload: %w", err)
	}
	helloBytes = append(helloBytes, payload...)

	// Parse ClientHello
	serverName = extractSNI(payload)
	return serverName, helloBytes, nil
}

// extractSNI parses a TLS handshake message to find the SNI extension.
func extractSNI(data []byte) string {
	if len(data) < 4 {
		return ""
	}

	// HandshakeType must be ClientHello (1)
	if data[0] != 1 {
		return ""
	}

	// Handshake length (3 bytes)
	handshakeLen := int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	if handshakeLen > len(data)-4 {
		handshakeLen = len(data) - 4
	}
	data = data[4 : 4+handshakeLen]

	// ClientVersion (2) + Random (32) = 34
	if len(data) < 34 {
		return ""
	}
	pos := 34

	// SessionID
	if pos >= len(data) {
		return ""
	}
	sessionIDLen := int(data[pos])
	pos += 1 + sessionIDLen

	// CipherSuites
	if pos+2 > len(data) {
		return ""
	}
	cipherLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2 + cipherLen

	// CompressionMethods
	if pos >= len(data) {
		return ""
	}
	compLen := int(data[pos])
	pos += 1 + compLen

	// Extensions
	if pos+2 > len(data) {
		return ""
	}
	extLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2

	extEnd := pos + extLen
	if extEnd > len(data) {
		extEnd = len(data)
	}

	for pos+4 <= extEnd {
		extType := int(data[pos])<<8 | int(data[pos+1])
		extDataLen := int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4

		if pos+extDataLen > extEnd {
			break
		}

		// SNI extension type = 0
		if extType == 0 {
			return parseSNIExtension(data[pos : pos+extDataLen])
		}

		pos += extDataLen
	}

	return ""
}

// parseSNIExtension extracts the hostname from an SNI extension payload.
func parseSNIExtension(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	listLen := int(data[0])<<8 | int(data[1])
	data = data[2:]
	if listLen > len(data) {
		listLen = len(data)
	}

	pos := 0
	for pos+3 <= listLen {
		nameType := data[pos]
		nameLen := int(data[pos+1])<<8 | int(data[pos+2])
		pos += 3
		if pos+nameLen > listLen {
			break
		}
		if nameType == 0 { // host_name
			return string(data[pos : pos+nameLen])
		}
		pos += nameLen
	}
	return ""
}
