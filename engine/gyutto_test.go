package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/gyutto"
)

func TestGyuttoRouting(t *testing.T) {
	e := New(nil)
	named, err := e.GetMovieProviderByName(gyutto.Name)
	require.NoError(t, err)
	_, ok := named.(provider.MovieSearcher)
	require.True(t, ok)
	for _, raw := range []string{"https://gyutto.com/i/item271421", "https://gyutto.com/i/item271421/af-13475?select_uaflag=1"} {
		p, err := e.GetMovieProviderByURL(raw)
		require.NoError(t, err)
		assert.Same(t, named, p)
	}
	assert.Len(t, e.movieHostProviders.GetOrDefault("gyutto.com", nil), 1)
	for _, raw := range []string{"https://gyutto.com.evil/i/item271421", "https://evilgyutto.com/i/item271421"} {
		_, err := e.GetMovieProviderByURL(raw)
		assert.Error(t, err)
	}
}
