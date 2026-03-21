package files

import (
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	// Register decoders
	_ "image/gif"

	"golang.org/x/image/draw"
)

// serveThumbnail generates and serves a thumbnail for an image file.
func serveThumbnail(w http.ResponseWriter, absPath string, maxW, maxH int) error {
	ext := strings.ToLower(filepath.Ext(absPath))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		// OK
	default:
		return errNotImage
	}

	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return err
	}

	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Calculate scaled dimensions preserving aspect ratio
	newW, newH := scaleDimensions(srcW, srcH, maxW, maxH)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	switch ext {
	case ".png":
		w.Header().Set("Content-Type", "image/png")
		return png.Encode(w, dst)
	default:
		w.Header().Set("Content-Type", "image/jpeg")
		return jpeg.Encode(w, dst, &jpeg.Options{Quality: 80})
	}
}

func scaleDimensions(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= maxW && srcH <= maxH {
		return srcW, srcH
	}

	ratioW := float64(maxW) / float64(srcW)
	ratioH := float64(maxH) / float64(srcH)

	ratio := ratioW
	if ratioH < ratioW {
		ratio = ratioH
	}

	return int(float64(srcW) * ratio), int(float64(srcH) * ratio)
}

type notImageError struct{}

func (notImageError) Error() string { return "not an image file" }

var errNotImage = notImageError{}
