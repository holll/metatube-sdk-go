package engine

import (
	"net/url"
	"testing"

	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/konodare"
)

type konodareDuplicateAliases struct{ *konodare.Konodare }

func (p *konodareDuplicateAliases) URLAliases() []*url.URL {
	upper, _ := url.Parse("https://KONODARE.JP/")
	alias := p.Konodare.URLAliases()[0]
	return []*url.URL{nil, {}, p.URL(), alias, upper, alias}
}

func TestKonodareAliasDedup(t *testing.T) {
	provider.Register(konodare.Name, func() *konodareDuplicateAliases { return &konodareDuplicateAliases{konodare.New()} })
	t.Cleanup(func() { provider.Register(konodare.Name, konodare.New) })
	e := New(nil)
	for _, host := range []string{"konodare.wew.jp", "konodare.jp"} {
		if ps := e.movieHostProviders.GetOrDefault(host, nil); len(ps) != 1 {
			t.Fatalf("%s registered %d times", host, len(ps))
		}
	}
}

func TestKonodareRouting(t *testing.T) {
	e := New(nil)
	named, err := e.GetMovieProviderByName(konodare.Name)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://konodare.wew.jp/pnme-333/", "https://konodare.jp/cosh-308/"} {
		p, err := e.GetMovieProviderByURL(raw)
		if err != nil || p != named {
			t.Fatal(raw, p, err)
		}
		u, _ := url.Parse(raw)
		if ps := e.movieHostProviders.GetOrDefault(u.Hostname(), nil); len(ps) != 1 {
			t.Fatalf("duplicate host registration: %v", ps)
		}
	}
	for _, raw := range []string{"https://konodare.jp.evil/cosh-308/", "https://evilkonodare.wew.jp/pnme-333/"} {
		if _, err := e.GetMovieProviderByURL(raw); err == nil {
			t.Fatal(raw)
		}
	}
}
