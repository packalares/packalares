package files

import (
	"net/http"

	"golang.org/x/net/webdav"
)

// NewWebDAVHandler creates a WebDAV handler serving files from dataPath.
func NewWebDAVHandler(dataPath string) http.Handler {
	return &webdav.Handler{
		Prefix:     "/webdav",
		FileSystem: webdav.Dir(dataPath),
		LockSystem: webdav.NewMemLS(),
	}
}
