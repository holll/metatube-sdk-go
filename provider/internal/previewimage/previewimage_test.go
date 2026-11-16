package previewimage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppend(t *testing.T) {
	base := "https://site.example/ja/video/movie"
	images := []string{}
	for _, raw := range []string{" /sample/1.jpg ", "https://site.example/sample/1.jpg#zoom", "//cdn.example/2.WEBP?token=x", "../3.png", "", "#", "data:image/gif;base64,AA", "javascript:alert(1)", "ftp://cdn.example/a.jpg", "https://user:pass@cdn.example/a.jpg", "https:///a.jpg", "https://cdn.example/a.mp4", "https://cdn.example/a.svg", "https://cdn.example/%ZZ.jpg", "/page"} {
		images = Append(images, raw, base)
	}
	assert.Equal(t, []string{"https://site.example/sample/1.jpg", "https://cdn.example/2.WEBP?token=x", "https://site.example/ja/3.png"}, images)
	assert.Empty(t, Append(nil, "/1.jpg", "://bad"))
}
