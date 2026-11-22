package glyph

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dotmatrx/yadokari/internal/wk"
)

// PickImage returns the URL and content type of the radical image to use.
// WaniKani serves only the SVG: the PNG entries are still listed but their
// URLs answer 403 (checked 2026-11-07), so a PNG is a last resort.
func PickImage(images []wk.CharacterImage) (url, contentType string) {
	for _, img := range images {
		if img.ContentType == "image/svg+xml" {
			return img.URL, img.ContentType
		}
	}
	best := 0
	for _, img := range images {
		if img.ContentType != "image/png" {
			continue
		}
		var meta struct {
			Dimensions string `json:"dimensions"`
		}
		_ = json.Unmarshal(img.Metadata, &meta)
		var px int
		fmt.Sscanf(meta.Dimensions, "%dx", &px)
		if url == "" || px > best {
			url, contentType, best = img.URL, img.ContentType, px
		}
	}
	return url, contentType
}

// ImageMask renders a downloaded radical image into a w×h mask.
func ImageMask(raw []byte, contentType string, w, h int) (*image.Alpha, error) {
	if contentType == "image/svg+xml" {
		return SVGMask(raw, w, h)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("glyph: %w", err)
	}
	return Mask(img, w, h), nil
}

// ImageCache downloads files once and keeps them on disk: radical images,
// and (in its own directory) pronunciation audio.
type ImageCache struct {
	Dir    string
	Client *http.Client
}

// Get returns the bytes at url, from disk if cached.
func (c ImageCache) Get(ctx context.Context, url string) ([]byte, error) {
	path, err := c.File(ctx, url)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// File returns the path of the cached file for url, downloading it first
// if needed.
func (c ImageCache) File(ctx context.Context, url string) (string, error) {
	sum := sha256.Sum256([]byte(url))
	path := filepath.Join(c.Dir, hex.EncodeToString(sum[:12]))
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("glyph: %s: HTTP %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return "", err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}
