// Package previewimage normalizes explicitly identified movie gallery URLs.
// Callers must scope candidates to the current movie; this package does not
// discover galleries or infer image URLs from covers or movie numbers.
package previewimage

import (
	"net/url"
	"slices"
	"strings"
)

// Append resolves an HTTP(S) image URL and appends it once, preserving order.
// Empty values, inline placeholders, credentials and non-image resources are
// ignored. No request is made to the image host.
func Append(images []string, raw, base string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return images
	}
	u, err := url.Parse(raw)
	if err != nil {
		return images
	}
	b, err := url.Parse(base)
	if err != nil {
		return images
	}
	u = b.ResolveReference(u)
	if (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return images
	}
	path := strings.ToLower(u.Path)
	if !slices.ContainsFunc([]string{".jpg", ".jpeg", ".png", ".webp", ".avif", ".gif"}, func(ext string) bool { return strings.HasSuffix(path, ext) }) {
		return images
	}
	u.Fragment = ""
	image := u.String()
	if !slices.Contains(images, image) {
		images = append(images, image)
	}
	return images
}
