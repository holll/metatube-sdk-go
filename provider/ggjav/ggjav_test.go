package ggjav

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/internal/testkit"
)

func TestGGJAV_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"301794", // KTRA-746
		"26848",  // sample from user
	})
}

func TestGGJAV_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{
		"SVDVD-809",
		"KTRA-746",
	})
}
