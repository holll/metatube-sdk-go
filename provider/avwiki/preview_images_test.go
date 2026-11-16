package avwiki

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The observed article-body images are recommendation covers; neither those
// nor the current product's cover constitute an independently supplied gallery.
func TestDetailDoesNotUseProductAndRecommendationCoversAsStills(t *testing.T) {
	body := strings.Replace(detail, `</section>`, `<aside class="product"><a href="https://example.com/store"><img src="https://images.example/current-jm.jpg"></a></aside>
<div class="col_4"><a><img data-src="https://images.example/other-cover.jpg" src="data:image/gif;base64,AA"></a></div></section>`, 1)
	info, err := fixed(t, 200, body).GetMovieInfoByID("HHSI-024")
	require.NoError(t, err)
	require.True(t, info.IsValid())
	assert.Empty(t, info.PreviewImages)
	assert.Equal(t, "https://images.example/current.jpg?w=300&h=300", info.CoverURL)
}
