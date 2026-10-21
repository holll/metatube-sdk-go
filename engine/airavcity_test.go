package engine

import (
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider/airavcity"
)

func TestAirAVCityRouting(t *testing.T) {
	e := New(nil)
	named, err := e.GetMovieProviderByName(airavcity.Name)
	if err != nil {
		t.Fatal(err)
	}
	// hid 详情 URL 必须能路由到本 provider，镜像域名同样要路由。
	for _, raw := range []string{
		"https://airavcity4.cc/video?hid=99-21-14639",
		"https://airavcity4.cc/video?hid=QC-BT-4734904",
		"https://airav.io/video?hid=99-21-14639",
	} {
		p, err := e.GetMovieProviderByURL(raw)
		if err != nil || p != named {
			t.Fatalf("%s -> %v, %v", raw, p, err)
		}
		id, err := p.ParseMovieIDFromURL(raw)
		if err != nil || id == "" {
			t.Fatalf("%s parse id: %q, %v", raw, id, err)
		}
	}
	for _, raw := range []string{
		"https://airavcity4.cc.evil/video?hid=99-21-14639",
		"https://evil-airavcity4.cc/video?hid=99-21-14639",
		"https://airav.io.evil/video?hid=99-21-14639",
		"https://evil-airav.io/video?hid=99-21-14639",
	} {
		if _, err := e.GetMovieProviderByURL(raw); err == nil {
			t.Fatalf("%s should not route to a provider", raw)
		}
	}
	// 两个域名只应各注册一次。
	for _, host := range []string{"airavcity4.cc", "airav.io"} {
		if ps := e.movieHostProviders.GetOrDefault(host, nil); len(ps) != 1 {
			t.Fatalf("%s registered %d times", host, len(ps))
		}
	}
}
