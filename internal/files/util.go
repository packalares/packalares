package files

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// uploadState tracks an in-progress chunked upload.
type uploadState struct {
	absPath  string
	fileSize int64
	offset   int64
}

var (
	uploadsMu        sync.Mutex
	uploadsInProgress = make(map[string]*uploadState)
)

// copyPath copies a file or directory tree from src to dst.
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	srcInfo, _ := os.Stat(src)
	return os.Chmod(dst, srcInfo.Mode())
}

func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// generateUniqueName appends a suffix to make a filename unique.
func generateUniqueName(path string) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)

	for i := 1; ; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// zipDir writes a zip archive of the directory to w.
func zipDir(dirPath string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	basePath := filepath.Dir(dirPath)

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			_, err := zw.Create(relPath + "/")
			return err
		}

		writer, err := zw.Create(relPath)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// detectFileType maps a filename to a frontend-compatible type string.
func detectFileType(name string, isDir bool) string {
	if isDir {
		return "directory"
	}

	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".svg", ".ico", ".tiff":
		return "image"
	case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v":
		return "video"
	case ".mp3", ".wav", ".flac", ".aac", ".ogg", ".wma", ".m4a", ".opus":
		return "audio"
	case ".pdf":
		return "pdf"
	case ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx":
		return "document"
	case ".zip", ".tar", ".gz", ".bz2", ".7z", ".rar", ".xz":
		return "archive"
	case ".txt", ".md", ".log", ".csv", ".json", ".xml", ".yaml", ".yml",
		".toml", ".ini", ".conf", ".cfg", ".sh", ".bash", ".zsh",
		".py", ".go", ".js", ".ts", ".html", ".css", ".sql",
		".rb", ".rs", ".java", ".c", ".cpp", ".h", ".hpp",
		".lua", ".php", ".pl", ".r", ".swift", ".kt", ".dart",
		".dockerfile", ".makefile", ".gitignore":
		return "text"
	default:
		return "blob"
	}
}

// isTextExt returns true if the extension is a text file type.
func isTextExt(ext string) bool {
	return detectFileType("file"+ext, false) == "text"
}
