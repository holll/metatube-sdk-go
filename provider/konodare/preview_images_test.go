package konodare

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetailDoesNotUseCosplayReferenceThumbnailAsStills(t *testing.T) {
	for _, tt := range []struct {
		name, id, body string
	}{
		{"modern", "jp:cosh-308", strings.Replace(newHTML, `<div class="mdb-work-content">`, `<div class="mdb-cosplay-list"><article class="mdb-cosplay-entry"><div class="mdb-cosplay-image-"><a class="mdb-cosplay-image-link" data-mdb-cosplay-lightbox="1" href="https://i0.wp.com/image.gyutto.com/data/item_img/2860/286079/286079_sc.jpg"><img src="https://i0.wp.com/image.gyutto.com/data/item_img/2860/286079/286079_sc.jpg?h=180"></a></div></article></div><img id="mdb-cosplay-lightbox-image" src=""><div class="mdb-work-content">`, 1)},
		{"legacy", "wew:pnme-333", strings.Replace(oldHTML, `<table class="cosplay-table">`, `<table class="cosplay-table"><tr><td class="cosplay-img"><img src="https://i0.wp.com/image.gyutto.com/data/item_img/2859/285907/285907_sc.jpg"></td></tr>`, 1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := offline(t, func(*http.Request) (int, string) { return 200, tt.body })
			info, err := p.GetMovieInfoByID(tt.id)
			require.NoError(t, err)
			require.True(t, info.IsValid())
			// The observed _sc.jpg is a 100x100 reference crop, not a movie
			// still gallery. Lightbox markup alone must not imply support.
			assert.Empty(t, info.PreviewImages)
		})
	}
}
