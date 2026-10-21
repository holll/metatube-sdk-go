package engine

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/fanza"
)

func TestFANZAVideoDomainRouting(t *testing.T) {
	e := New(nil)
	named, err := e.GetMovieProviderByName(fanza.Name)
	if err != nil {
		t.Fatal(err)
	}
	// video.dmm.co.jp 必须路由到 FANZA，否则引擎无法按新站 URL 取详情。
	for _, raw := range []string{
		"https://video.dmm.co.jp/av/content/?id=ssis00001",
		"https://video.dmm.co.jp/amateur/content/?id=hhsi024",
		"https://www.dmm.co.jp/digital/videoa/-/detail/=/cid=hhsi024/",
	} {
		p, err := e.GetMovieProviderByURL(raw)
		if err != nil || p != named {
			t.Fatalf("%s -> %v, %v", raw, p, err)
		}
	}
	if ps := e.movieHostProviders.GetOrDefault("video.dmm.co.jp", nil); len(ps) != 1 {
		t.Fatalf("video.dmm.co.jp registered %d times", len(ps))
	}
	for _, raw := range []string{"https://video.dmm.co.jp.evil/av/content/?id=x", "https://evilvideo.dmm.co.jp/av/content/?id=x"} {
		if _, err := e.GetMovieProviderByURL(raw); err == nil {
			t.Fatalf("%s should not route to a provider", raw)
		}
	}
}
