package engine

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/avgood"
)

func TestAvGoodRouting(t *testing.T) {
	e := New(nil)
	named, err := e.GetMovieProviderByName(avgood.Name)
	if err != nil {
		t.Fatal(err)
	}
	if named.Priority() != avgood.Priority {
		t.Fatal(named.Priority())
	}
	for _, raw := range []string{"https://avgood.com/j/53247.html", "https://avgood.com/c/105297.html", "https://avgood.com/e/53247.html"} {
		p, err := e.GetMovieProviderByURL(raw)
		if err != nil || p != named {
			t.Fatal(raw, p, err)
		}
	}
	if _, err := e.GetMovieProviderByURL("https://avgood.com.evil/j/53247.html"); err == nil {
		t.Fatal("错误接受伪造域名")
	}
}
