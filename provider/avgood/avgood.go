package avgood

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

const (
	Name     = "AvGood"
	Priority = 300
	baseURL  = "https://avgood.com/"
)

var (
	_                  provider.MovieProvider = (*AvGood)(nil)
	_                  provider.MovieSearcher = (*AvGood)(nil)
	idPattern                                 = regexp.MustCompile(`^[1-9][0-9]*$`)
	pathPattern                               = regexp.MustCompile(`^/[cejk]/([1-9][0-9]*)\.html$`)
	numberPattern                             = regexp.MustCompile(`^[A-Z0-9]+(?:[-_][A-Z0-9]+)*$`)
	searchTitlePattern                        = regexp.MustCompile(`(?i)^([a-z0-9]+(?:\s*[-_]\s*[a-z0-9]+)*)\s+(.+)$`)
)

type AvGood struct{ *scraper.Scraper }

func New() *AvGood {
	p := &AvGood{scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese)}
	p.ClonedCollector().SetRedirectHandler(func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("%s: too many redirects", Name)
		}
		if !strings.HasPrefix(r.URL.Path, "/j/") {
			return provider.ErrInvalidURL
		}
		return p.validateURL(r.URL, len(via) > 0 && via[0].URL.Path == "/j/s/")
	})
	return p
}

func (p *AvGood) NormalizeMovieKeyword(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

func (p *AvGood) NormalizeMovieID(s string) string { return p.NormalizeMovieKeyword(s) }

func (p *AvGood) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", provider.ErrInvalidURL
	}
	if err = p.validateURL(u, false); err != nil {
		return "", err
	}
	return pathPattern.FindStringSubmatch(u.Path)[1], nil
}

func (p *AvGood) validateURL(u *url.URL, search bool) error {
	if (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, "avgood.com") || u.User != nil || u.RawPath != "" || u.Fragment != "" || u.ForceQuery {
		return provider.ErrInvalidURL
	}
	if search {
		if u.Path != "/j/s/" || len(u.Query()) != 1 || len(u.Query()["q"]) != 1 || strings.TrimSpace(u.Query().Get("q")) == "" {
			return provider.ErrInvalidURL
		}
	} else if u.RawQuery != "" || !pathPattern.MatchString(u.Path) {
		return provider.ErrInvalidURL
	}
	return nil
}

func (p *AvGood) document(raw string, search bool) (*goquery.Document, *url.URL, error) {
	c := p.ClonedCollector()
	var doc *goquery.Document
	var final *url.URL
	var parseErr error
	c.OnResponse(func(r *colly.Response) {
		final = r.Request.URL
		if parseErr = p.validateURL(final, search); parseErr != nil {
			return
		}
		doc, parseErr = goquery.NewDocumentFromReader(strings.NewReader(string(r.Body)))
		if parseErr != nil {
			return
		}
		title := strings.ToLower(doc.Find("title").Text())
		if doc.Find("#challenge-form, #cf-challenge-running, .g-recaptcha, #captcha").Length() > 0 || strings.Contains(title, "just a moment") || strings.Contains(title, "captcha") || strings.Contains(title, "access denied") {
			parseErr = fmt.Errorf("%s: access challenge", Name)
		}
	})
	status := 0
	c.OnError(func(r *colly.Response, _ error) { status = r.StatusCode })
	if err := c.Visit(raw); err != nil {
		if status == http.StatusNotFound {
			return nil, nil, provider.ErrInfoNotFound
		}
		return nil, nil, err
	}
	if parseErr != nil {
		return nil, nil, parseErr
	}
	if doc == nil {
		return nil, nil, fmt.Errorf("%s: empty response", Name)
	}
	return doc, final, nil
}

func sameNumber(a, b string) bool {
	key := func(s string) string {
		return strings.NewReplacer("-", "", "_", "").Replace(strings.ToUpper(strings.Join(strings.Fields(s), "")))
	}
	return a != "" && b != "" && key(a) == key(b)
}

func (p *AvGood) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	id = p.NormalizeMovieID(id)
	if idPattern.MatchString(id) {
		return p.GetMovieInfoByURL(baseURL + "j/" + id + ".html")
	}
	if !numberPattern.MatchString(id) || !strings.ContainsAny(id, "0123456789") || !strings.ContainsAny(id, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return nil, provider.ErrInvalidID
	}
	rs, err := p.SearchMovie(id)
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		if !sameNumber(r.Number, id) {
			continue
		}
		m, err := p.GetMovieInfoByURL(r.Homepage)
		if err != nil {
			return nil, err
		}
		if !sameNumber(m.Number, id) {
			return nil, provider.ErrInfoNotFound
		}
		return m, nil
	}
	return nil, provider.ErrInfoNotFound
}

func mediaURL(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	u = base.ResolveReference(u)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func (p *AvGood) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	id, err := p.ParseMovieIDFromURL(raw)
	if err != nil {
		return nil, err
	}
	// 搜索入口跨语言返回链接，统一读取日文详情，避免混入中文机翻字段。
	home := baseURL + "j/" + id + ".html"
	doc, final, err := p.document(home, false)
	if err != nil {
		return nil, err
	}
	if final.Path != "/j/"+id+".html" {
		return nil, provider.ErrInfoNotFound
	}
	m := &model.MovieInfo{ID: id, Provider: Name, Homepage: home, Actors: []string{}, Genres: []string{}, PreviewImages: []string{}}
	m.Title = strings.TrimSpace(doc.Find("h1.content-title").First().Text())
	info := doc.Find(".content-info").First()
	info.ChildrenFiltered("p").Each(func(_ int, row *goquery.Selection) {
		key := strings.Trim(strings.TrimSpace(row.ChildrenFiltered(".header").First().Text()), ":： ")
		value := row.Clone()
		value.Find(".header").Remove()
		s := strings.TrimSpace(value.Text())
		switch key {
		case "識別子":
			m.Number = p.NormalizeMovieKeyword(s)
		case "発行日":
			m.ReleaseDate = parser.ParseDate(s)
		case "長さ":
			m.Runtime = parser.ParseRuntime(s)
		case "メーカー":
			m.Maker = s
		case "出版社":
			m.Label = s
		case "監督":
			m.Director = s
		case "シリーズ":
			m.Series = s
		}
	})
	info.Find(".star-name a").Each(func(_ int, a *goquery.Selection) { m.Actors = appendUnique(m.Actors, strings.TrimSpace(a.Text())) })
	// 演员行也使用 .genre，必须排除，避免将演员重复写入类别。
	info.Find(".genre:not([onmouseover]) a").Each(func(_ int, a *goquery.Selection) { m.Genres = appendUnique(m.Genres, strings.TrimSpace(a.Text())) })
	cover := doc.Find("a.bigImage").First()
	m.CoverURL = mediaURL(cover.AttrOr("href", ""), final)
	if m.CoverURL == "" {
		img := cover.Find("img").First()
		for _, key := range []string{"data-original", "data-src", "src"} {
			if m.CoverURL = mediaURL(img.AttrOr(key, ""), final); m.CoverURL != "" {
				break
			}
		}
	}
	doc.Find("a.sample-box[href]").Each(func(_ int, a *goquery.Selection) {
		m.PreviewImages = appendUnique(m.PreviewImages, mediaURL(a.AttrOr("href", ""), final))
	})
	m.ThumbURL, m.BigCoverURL, m.BigThumbURL = m.CoverURL, m.CoverURL, m.CoverURL
	if !m.IsValid() || !numberPattern.MatchString(m.Number) || !strings.ContainsAny(m.Number, "0123456789") {
		return nil, provider.ErrIncompleteMetadata
	}
	return m, nil
}

func appendUnique(ss []string, s string) []string {
	if s == "" {
		return ss
	}
	for _, existing := range ss {
		if existing == s {
			return ss
		}
	}
	return append(ss, s)
}

func (p *AvGood) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	if strings.TrimSpace(keyword) == "" {
		return nil, provider.ErrInvalidKeyword
	}
	doc, base, err := p.document(baseURL+"j/s/?"+url.Values{"q": {keyword}}.Encode(), true)
	if err != nil {
		return nil, err
	}
	cards := doc.Find("h3.result-title > a[href]")
	results := []*model.MovieSearchResult{}
	if cards.Length() == 0 {
		if doc.Find(".container .search-info").Length() > 0 && strings.Contains(doc.Find(".container .no-results").Text(), "関連結果が見つかりません") {
			return results, nil
		}
		return nil, fmt.Errorf("%s: unrecognized search template", Name)
	}
	seen := map[string]bool{}
	var invalid bool
	cards.Each(func(_ int, a *goquery.Selection) {
		u, e := url.Parse(strings.TrimSpace(a.AttrOr("href", "")))
		if e != nil {
			invalid = true
			return
		}
		id, e := p.ParseMovieIDFromURL(base.ResolveReference(u).String())
		parts := searchTitlePattern.FindStringSubmatch(strings.TrimSpace(a.Text()))
		if e != nil || len(parts) != 3 {
			invalid = true
			return
		}
		num := p.NormalizeMovieKeyword(parts[1])
		if !strings.ContainsAny(num, "0123456789") {
			invalid = true
			return
		}
		if seen[id] {
			return
		}
		seen[id] = true
		results = append(results, &model.MovieSearchResult{ID: id, Provider: Name, Number: num, Title: strings.TrimSpace(parts[2]), Homepage: baseURL + "j/" + id + ".html"})
	})
	if invalid {
		return nil, provider.ErrIncompleteMetadata
	}
	return results, nil
}

func init() { provider.Register(Name, New) }
