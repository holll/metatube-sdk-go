package gyutto

import (
	"bytes"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

const (
	Name     = "Gyutto"
	Priority = 1000
	baseURL  = "https://gyutto.com/"
)

var (
	_              provider.MovieProvider = (*Gyutto)(nil)
	_              provider.MovieSearcher = (*Gyutto)(nil)
	ErrChallenge                          = errors.New("Gyutto: request verification required")
	inputPattern                          = regexp.MustCompile(`(?i)^(?:GYUTTO[-_])?([1-9][0-9]*)$`)
	pathPattern                           = regexp.MustCompile(`^/i/item([1-9][0-9]*)(?:/af-[1-9][0-9]*)?/?$`)
	runtimePattern                        = regexp.MustCompile(`(?:動画[：:]\s*|収録分数\s*約?\s*)([0-9]+)\s*分`)
)

type Gyutto struct{ *scraper.Scraper }

func New() *Gyutto {
	p := &Gyutto{scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese, scraper.WithDetectCharset())}
	p.ClonedCollector().SetRedirectHandler(func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return provider.ErrInvalidURL
		}
		search := len(via) > 0 && via[0].URL.Path == "/search/search_list.php"
		return p.validateURL(r.URL, search)
	})
	return p
}

func (p *Gyutto) NormalizeMovieID(id string) string {
	if match := inputPattern.FindStringSubmatch(strings.TrimSpace(id)); len(match) == 2 {
		return match[1]
	}
	return ""
}

func siteURL(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, "gyutto.com") && u.User == nil && u.Fragment == "" && u.RawFragment == "" && u.Opaque == "" && !strings.Contains(u.EscapedPath(), "%")
}

func (p *Gyutto) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !siteURL(u) || u.ForceQuery {
		return "", provider.ErrInvalidURL
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", provider.ErrInvalidURL
	}
	for key, vs := range q {
		if key != "select_uaflag" || len(vs) != 1 || vs[0] != "1" {
			return "", provider.ErrInvalidURL
		}
	}
	if match := pathPattern.FindStringSubmatch(u.Path); len(match) == 2 {
		return match[1], nil
	}
	return "", provider.ErrInvalidURL
}

func (p *Gyutto) validateURL(u *url.URL, search bool) error {
	if !search {
		_, err := p.ParseMovieIDFromURL(u.String())
		return err
	}
	if !siteURL(u) || u.Path != "/search/search_list.php" {
		return provider.ErrInvalidURL
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || q.Get("mode") != "search" || q.Get("category_id") != "10" || q.Get("search_keyword") == "" {
		return provider.ErrInvalidURL
	}
	return nil
}

func (p *Gyutto) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	id = p.NormalizeMovieID(id)
	if id == "" {
		return nil, provider.ErrInvalidID
	}
	return p.GetMovieInfoByURL(baseURL + "i/item" + id)
}

func challenge(doc *goquery.Document) bool {
	title := strings.ToLower(strings.TrimSpace(doc.Find("title").Text()))
	return doc.Find("#challenge-form, #cf-challenge-running, .g-recaptcha, #captcha").Length() > 0 || strings.Contains(title, "just a moment") || strings.Contains(title, "captcha") || strings.Contains(title, "access denied") || strings.Contains(title, "年齢認証") || strings.Contains(title, "年齢確認")
}

func (p *Gyutto) document(raw string, search bool) (*goquery.Document, *url.URL, error) {
	c := p.ClonedCollector()
	var doc *goquery.Document
	var final *url.URL
	var responseErr error
	c.OnResponse(func(r *colly.Response) {
		final = r.Request.URL
		if responseErr = p.validateURL(final, search); responseErr != nil {
			return
		}
		doc, responseErr = goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
		if responseErr == nil && challenge(doc) {
			responseErr = ErrChallenge
		}
	})
	c.OnError(func(r *colly.Response, err error) {
		responseErr = err
		if r == nil {
			return
		}
		if d, e := goquery.NewDocumentFromReader(bytes.NewReader(r.Body)); e == nil && challenge(d) {
			responseErr = ErrChallenge
		} else if r.StatusCode == http.StatusNotFound {
			responseErr = provider.ErrInfoNotFound
		}
	})
	visitErr := c.Visit(raw)
	if responseErr != nil {
		return nil, nil, responseErr
	}
	if visitErr != nil {
		return nil, nil, visitErr
	}
	if doc == nil || final == nil {
		return nil, nil, provider.ErrIncompleteMetadata
	}
	return doc, final, nil
}

func (p *Gyutto) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	id, err := p.ParseMovieIDFromURL(raw)
	if err != nil {
		return nil, err
	}
	// 不将推广参数写入请求或存储的作品地址，固定使用 PC 版结构。
	doc, base, err := p.document(baseURL+"i/item"+id+"?select_uaflag=1", false)
	if err != nil {
		return nil, err
	}
	finalID, err := p.ParseMovieIDFromURL(base.String())
	if err != nil {
		return nil, err
	}
	if finalID != id {
		return nil, provider.ErrInfoNotFound
	}
	if raw, ok := doc.Find(`meta[property="og:url"]`).First().Attr("content"); ok {
		u, e := url.Parse(strings.TrimSpace(raw))
		if e != nil {
			return nil, provider.ErrInvalidURL
		}
		canonicalID, e := p.ParseMovieIDFromURL(base.ResolveReference(u).String())
		if e != nil {
			return nil, e
		}
		if canonicalID != id {
			return nil, provider.ErrInfoNotFound
		}
	}
	m := &model.MovieInfo{ID: id, Number: "GYUTTO-" + id, Provider: Name, Homepage: baseURL + "i/item" + id, Actors: []string{}, Genres: []string{}, PreviewImages: []string{}}
	body := doc.Find("#RightSide").First()
	m.Title = strings.TrimSpace(body.Find(".parts_Mds01 h1").First().Text())
	m.CoverURL = mediaURL(body.Find(".unit_DojinMainPh .ItemPh img").First().AttrOr("src", ""), base)
	if m.CoverURL == "" {
		m.CoverURL = mediaURL(doc.Find(`meta[property="og:image"]`).First().AttrOr("content", ""), base)
	}
	body.Find(".unit_DetailBasicInfo").First().Find("dl.BasicInfo").Each(func(_ int, dl *goquery.Selection) {
		v := dl.ChildrenFiltered("dd").First()
		switch strings.TrimSpace(dl.ChildrenFiltered("dt").First().Text()) {
		case "サークル":
			m.Label = strings.TrimSpace(v.Find("a").First().Text())
		case "配信開始日":
			m.ReleaseDate = parser.ParseDate(strings.TrimSpace(v.Text()))
		case "ジャンル":
			v.Find("a").Each(func(_ int, a *goquery.Selection) { m.Genres = appendUnique(m.Genres, strings.TrimSpace(a.Text())) })
		}
	})
	heading := body.Find(".parts_Mds02").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.TrimSpace(s.Find("h2").Text()) == "作品概要"
	}).First()
	m.Summary = summary(heading.NextFiltered(".unit_DetailSummary"))
	description := doc.Find(`meta[property="og:description"]`).First().AttrOr("content", "")
	m.Runtime = parser.ParseRuntime(runtimePattern.FindString(description))
	if m.Runtime == 0 {
		m.Runtime = parser.ParseRuntime(runtimePattern.FindString(m.Summary))
	}
	body.Find(".unit_SamplePhSmall a.highslide[href]").Each(func(_ int, a *goquery.Selection) {
		m.PreviewImages = appendUnique(m.PreviewImages, mediaURL(a.AttrOr("href", ""), base))
	})
	m.ThumbURL, m.BigCoverURL, m.BigThumbURL = m.CoverURL, m.CoverURL, m.CoverURL
	if !m.IsValid() {
		return nil, provider.ErrIncompleteMetadata
	}
	return m, nil
}

func (p *Gyutto) NormalizeMovieKeyword(keyword string) string { return strings.TrimSpace(keyword) }

func (p *Gyutto) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	keyword = p.NormalizeMovieKeyword(keyword)
	if keyword == "" {
		return nil, provider.ErrInvalidKeyword
	}
	encoded, err := japanese.EUCJP.NewEncoder().String(keyword)
	if err != nil {
		return nil, provider.ErrInvalidKeyword
	}
	q := url.Values{"category_id": {"10"}, "set_category_flag": {"1"}, "mode": {"search"}, "select_uaflag": {"1"}, "search_keyword": {encoded}}
	doc, base, err := p.document(baseURL+"search/search_list.php?"+q.Encode(), true)
	if err != nil {
		return nil, err
	}
	cards := doc.Find("#struct_2ColRightIn .unit_ItemList .unit_SeachSmall .ListBox dl.ItemBox")
	results := []*model.MovieSearchResult{}
	if cards.Length() == 0 {
		empty := strings.TrimSpace(doc.Find("#struct_2ColRightIn > .parts_Lead > p > .Alert").First().Text())
		if empty == "アダルト作品では、「"+keyword+"」に一致する作品はありませんでした。" {
			return results, nil
		}
		return nil, provider.ErrIncompleteMetadata
	}
	seen := map[string]bool{}
	invalid := false
	cards.Each(func(_ int, card *goquery.Selection) {
		a := card.Find(".DefiPhotoName a[href]").First()
		href := strings.TrimSpace(a.AttrOr("href", ""))
		u, e := url.Parse(href)
		if e != nil || href == "" {
			invalid = true
			return
		}
		id, e := p.ParseMovieIDFromURL(base.ResolveReference(u).String())
		if e != nil {
			invalid = true
			return
		}
		title := strings.TrimSpace(a.Find("img").First().AttrOr("alt", ""))
		if title == "" {
			title = strings.TrimSpace(a.Text())
		}
		if title == "" {
			invalid = true
			return
		}
		if seen[id] {
			return
		}
		image := mediaURL(a.Find("img").First().AttrOr("src", ""), base)
		results = append(results, &model.MovieSearchResult{ID: id, Number: "GYUTTO-" + id, Title: title, Provider: Name, Homepage: baseURL + "i/item" + id, CoverURL: image, ThumbURL: image})
		seen[id] = true
	})
	if invalid {
		return nil, provider.ErrIncompleteMetadata
	}
	return results, nil
}

func mediaURL(raw string, base *url.URL) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	u = base.ResolveReference(u)
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func appendUnique(out []string, s string) []string {
	if s == "" {
		return out
	}
	for _, v := range out {
		if v == s {
			return out
		}
	}
	return append(out, s)
}

func summary(s *goquery.Selection) string {
	copy := s.Clone()
	copy.Find("br").ReplaceWithHtml("\n")
	copy.Find("p").PrependHtml("\n").AppendHtml("\n")
	var lines []string
	for _, line := range strings.Split(copy.Text(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func init() { provider.Register(Name, New) }
