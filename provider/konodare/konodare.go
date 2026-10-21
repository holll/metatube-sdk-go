package konodare

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

const (
	Name     = "Konodare"
	Priority = 400
	oldBase  = "https://konodare.wew.jp/"
	newBase  = "https://konodare.jp/"
)

var (
	_             provider.MovieProvider      = (*Konodare)(nil)
	_             provider.MovieSearcher      = (*Konodare)(nil)
	_             provider.ProviderURLAliases = (*Konodare)(nil)
	slugPattern                               = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	numberPattern                             = regexp.MustCompile(`^[A-Z0-9]+(?:-[A-Z0-9]+)*$`)
	datePattern                               = regexp.MustCompile(`\d{4}[/.年-]\d{1,2}[/.月-]\d{1,2}日?`)
)

type Konodare struct{ *scraper.Scraper }

func New() *Konodare {
	p := &Konodare{scraper.NewDefaultScraper(Name, oldBase, Priority, language.Japanese)}
	// Collector 克隆共享 HTTP 客户端，因此仅设置一次重定向策略，
	// 避免并发查询时修改共享客户端。
	p.ClonedCollector().SetRedirectHandler(func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("%s: too many redirects", Name)
		}
		search := len(via) > 0 && via[0].URL.Query().Has("s")
		return p.validateRequestURL(r.URL, search)
	})
	return p
}
func (p *Konodare) URLAliases() []*url.URL { u, _ := url.Parse(newBase); return []*url.URL{u} }
func (p *Konodare) NormalizeMovieKeyword(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}
func (p *Konodare) NormalizeMovieID(s string) string {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ":") {
		return strings.ToLower(s)
	}
	return p.NormalizeMovieKeyword(s)
}
func validSlug(s string) bool {
	// 作品 slug 必须包含字母和数字，排除纯数字归档及常见分类、WordPress 端点。
	return slugPattern.MatchString(s) && strings.ContainsAny(s, "0123456789") && strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz")
}
func (p *Konodare) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return "", provider.ErrInvalidURL
	}
	prefix := ""
	switch strings.ToLower(u.Host) {
	case "konodare.wew.jp":
		prefix = "wew:"
	case "konodare.jp":
		prefix = "jp:"
	default:
		return "", provider.ErrInvalidURL
	}
	slug := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/")
	if !validSlug(slug) || u.Path != "/"+slug && u.Path != "/"+slug+"/" {
		return "", provider.ErrInvalidURL
	}
	return prefix + slug, nil
}
func (p *Konodare) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	id = p.NormalizeMovieID(id)
	if strings.Contains(id, ":") {
		parts := strings.Split(id, ":")
		if len(parts) != 2 || !validSlug(parts[1]) {
			return nil, provider.ErrInvalidID
		}
		base := oldBase
		switch parts[0] {
		case "wew":
		case "jp":
			base = newBase
		default:
			return nil, provider.ErrInvalidID
		}
		return p.GetMovieInfoByURL(base + parts[1] + "/")
	}
	if !numberPattern.MatchString(id) || !strings.ContainsAny(id, "0123456789") {
		return nil, provider.ErrInvalidID
	}
	rs, err := p.SearchMovie(id)
	if err != nil {
		return nil, err
	}
	// 仅在番号比较时忽略横杠、大小写和空白，保留数字前导零及返回格式。
	sameNumber := func(a, b string) bool {
		return strings.ReplaceAll(p.NormalizeMovieKeyword(a), "-", "") ==
			strings.ReplaceAll(p.NormalizeMovieKeyword(b), "-", "")
	}
	for _, r := range rs {
		if sameNumber(r.Number, id) {
			m, err := p.GetMovieInfoByURL(r.Homepage)
			if err != nil {
				return nil, err
			}
			if !sameNumber(m.Number, id) {
				return nil, provider.ErrInfoNotFound
			}
			return m, nil
		}
	}
	return nil, provider.ErrInfoNotFound
}

func (p *Konodare) validateRequestURL(u *url.URL, search bool) error {
	if !search {
		_, err := p.ParseMovieIDFromURL(u.String())
		return err
	}
	host := strings.ToLower(u.Host)
	if (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Port() != "" || (host != "konodare.wew.jp" && host != "konodare.jp") || u.Path != "/" || u.Query().Get("s") == "" {
		return provider.ErrInvalidURL
	}
	return nil
}

// document 沿用共享 scraper 的传输、代理和超时设置。
func (p *Konodare) document(raw string, search bool) (*goquery.Document, *url.URL, error) {
	c := p.ClonedCollector()
	var doc *goquery.Document
	var final *url.URL
	var parseErr error
	c.OnResponse(func(r *colly.Response) {
		final = r.Request.URL
		if parseErr = p.validateRequestURL(final, search); parseErr != nil {
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
	err := c.Visit(raw)
	if err != nil {
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
func text(s *goquery.Selection) string { return strings.TrimSpace(s.Text()) }

// summary 保留正文中的换行和段落边界，不修改原始 DOM。
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

func label(s string) string { return strings.Trim(strings.TrimSpace(s), ":： \t\r\n") }
func values(s *goquery.Selection) []string {
	var out []string
	if s.Find("a").Length() > 0 {
		s.Find("a").Each(func(_ int, a *goquery.Selection) { out = append(out, text(a)) })
	} else {
		out = append(out, text(s))
	}
	return unique(out)
}
func unique(ss []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" && s != "---" && s != "―" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
func image(s *goquery.Selection, base *url.URL) string {
	result := ""
	s.EachWithBreak(func(_ int, img *goquery.Selection) bool {
		for _, key := range []string{"data-src", "src"} {
			v := strings.TrimSpace(img.AttrOr(key, ""))
			if v == "" {
				continue
			}
			u, err := url.Parse(v)
			if err != nil {
				continue
			}
			u = base.ResolveReference(u)
			if (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
				result = u.String()
				return false
			}
		}
		return true
	})
	return result
}
func field(m *model.MovieInfo, key string, v *goquery.Selection) {
	s := text(v)
	if s == "---" || s == "―" {
		return
	}
	switch label(key) {
	case "品番", "番号":
		if m.Number == "" {
			m.Number = s
		}
	case "女優", "出演者":
		m.Actors = unique(append(m.Actors, values(v)...))
	case "メーカー":
		m.Maker = s
	case "サークル", "レーベル":
		m.Label = s
		if m.Maker == "" {
			m.Maker = s
		}
	case "シリーズ":
		m.Series = s
	case "配信日", "発売日", "発売日付":
		if time.Time(m.ReleaseDate).IsZero() {
			m.ReleaseDate = parser.ParseDate(s)
		}
	case "時間", "収録時間":
		if m.Runtime == 0 {
			m.Runtime = parser.ParseRuntime(s)
		}
	}
}
func (p *Konodare) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	if _, err := p.ParseMovieIDFromURL(raw); err != nil {
		return nil, err
	}
	doc, base, err := p.document(raw, false)
	if err != nil {
		return nil, err
	}
	home := base.String()
	if canonical := doc.Find(`link[rel="canonical"]`).First(); canonical.Length() > 0 {
		href := strings.TrimSpace(canonical.AttrOr("href", ""))
		if href == "" {
			return nil, provider.ErrInvalidURL
		}
		u, e := url.Parse(href)
		if e != nil {
			return nil, provider.ErrInvalidURL
		}
		home = base.ResolveReference(u).String()
	}
	id, err := p.ParseMovieIDFromURL(home)
	if err != nil {
		return nil, err
	}
	m := &model.MovieInfo{ID: id, Provider: Name, Homepage: home, Actors: []string{}, Genres: []string{}, PreviewImages: []string{}}
	a := doc.Find("article.mdb-work").First()
	if a.Length() > 0 {
		m.Title = text(a.Find(".mdb-work-title").First())
		m.Number = text(a.Find(".mdb-work-item-no").First())
		m.Summary = summary(a.Find(".mdb-work-content").First())
		a.Find("video source[src]").EachWithBreak(func(_ int, source *goquery.Selection) bool {
			raw := strings.TrimSpace(source.AttrOr("src", ""))
			if raw == "" {
				return true
			}
			u, err := url.Parse(raw)
			if err != nil {
				return true
			}
			u = base.ResolveReference(u)
			if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
				return true
			}
			m.PreviewVideoURL = u.String()
			return false
		})
		m.CoverURL = image(a.Find(".mdb-work-cover img"), base)
		a.Find(".mdb-summary-row").Each(func(_ int, r *goquery.Selection) {
			field(m, text(r.Find(".mdb-summary-key")), r.Find(".mdb-summary-value"))
		})
		a.Find(".mdb-source-card").Each(func(_ int, card *goquery.Selection) {
			card.Find(".mdb-source-meta-row").Each(func(_ int, r *goquery.Selection) { field(m, text(r.Find("span").First()), r.Find("strong").First()) })
		})
		a.Find(".mdb-cosplay-meta-row").Each(func(_ int, r *goquery.Selection) {
			key := label(text(r.Find(".mdb-cosplay-label")))
			v := r.Find(".mdb-cosplay-value")
			if key == "女優" {
				field(m, key, v)
			} else if key == "キャラクター" || key == "出典" || key == "原作" {
				m.Genres = append(m.Genres, values(v)...)
			}
		})
	} else {
		a = doc.Find("article").FilterFunction(func(_ int, s *goquery.Selection) bool { return s.Find(".entry-content").Length() > 0 }).First()
		body := a.Find(".entry-content").First()
		m.Title = text(body.Find("h9").First())
		m.Summary = summary(body.Find("h9").First().NextAllFiltered("div").First())
		product := body.Find("table.product").First()
		m.Number = text(product.Find("th").First())
		if m.Number == "" {
			m.Number = text(a.Find("h1").First())
		}
		m.CoverURL = image(product.Find(".product_image img, tr.mini img"), base)
		product.Find("td.product_item").Each(func(_ int, r *goquery.Selection) { field(m, text(r), r.NextFiltered("td")) })
		body.Find(".cosplay-table td.chara").Each(func(_ int, r *goquery.Selection) {
			key := label(text(r))
			if key == "キャラクター" || key == "出典" || key == "原作" {
				m.Genres = append(m.Genres, values(r.NextFiltered("td"))...)
			}
		})
	}
	m.Number = p.NormalizeMovieKeyword(m.Number)
	m.Genres = unique(m.Genres)
	m.ThumbURL = m.CoverURL
	m.BigCoverURL = m.CoverURL
	m.BigThumbURL = m.CoverURL
	if !m.IsValid() {
		return nil, provider.ErrIncompleteMetadata
	}
	return m, nil
}
func (p *Konodare) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	if strings.TrimSpace(keyword) == "" {
		return nil, provider.ErrInvalidKeyword
	}
	rs, err := p.searchSite(oldBase, keyword)
	if err != nil || len(rs) > 0 {
		return rs, err
	}
	return p.searchSite(newBase, keyword)
}
func (p *Konodare) searchSite(base, keyword string) ([]*model.MovieSearchResult, error) {
	doc, u, err := p.document(base+"?"+url.Values{"s": {keyword}}.Encode(), true)
	if err != nil {
		return nil, err
	}
	modern := strings.EqualFold(u.Hostname(), "konodare.jp")
	container, cards := "#list", "#list > a.entry-card-wrap"
	if modern {
		container = ".mdb-search-grid"
		cards = ".mdb-search-grid > article.mdb-card"
	}
	if doc.Find(container).Length() == 0 {
		// 仅接受搜索主区域内的已知空结果模板，不能凭页面任意位置的提示判断。
		if modern {
			empty := doc.Find("main.mdb-main > div.mdb-empty").First()
			if text(empty) == "「"+keyword+"」に一致する作品が見つかりませんでした。" {
				return []*model.MovieSearchResult{}, nil
			}
		} else {
			empty := doc.Find("body.search-no-results main#main div.posts-not-found > p").First()
			if text(empty) == "投稿が見つかりませんでした。" {
				return []*model.MovieSearchResult{}, nil
			}
		}
		return nil, fmt.Errorf("%s: unrecognized search template", Name)
	}
	results := []*model.MovieSearchResult{}
	seen := map[string]bool{}
	var invalid bool
	doc.Find(cards).Each(func(_ int, card *goquery.Selection) {
		r := &model.MovieSearchResult{Provider: Name}
		href := card.AttrOr("href", "")
		if modern {
			href = card.Find(".mdb-card-title a").First().AttrOr("href", "")
			r.Title = text(card.Find(".mdb-card-title"))
			r.Number = labelValue(text(card.Find(".mdb-card-id")), "品番")
			r.ReleaseDate = parser.ParseDate(labelValue(text(card.Find(".mdb-card-date")), "発売日"))
			r.Actors = values(card.Find(".mdb-card-actress"))
		} else {
			card = card.ChildrenFiltered("article").First()
			if card.Length() == 0 {
				invalid = true
				return
			}
			r.Title = text(card.Find("b.entry-card-title"))
			r.Number = text(card.Find(".card-number"))
			r.ReleaseDate = parser.ParseDate(datePattern.FindString(text(card.Find(".product_item"))))
			card.Find(`a[href*="/actress/"]`).Each(func(_ int, a *goquery.Selection) {
				r.Actors = append(r.Actors, text(a))
			})
			r.Actors = unique(r.Actors)
		}
		if href == "" {
			invalid = true
			return
		}
		ref, e := url.Parse(href)
		if e != nil {
			invalid = true
			return
		}
		r.Homepage = u.ResolveReference(ref).String()
		r.ID, e = p.ParseMovieIDFromURL(r.Homepage)
		r.Number = p.NormalizeMovieKeyword(r.Number)
		r.CoverURL = image(card.Find("img"), u)
		r.ThumbURL = r.CoverURL
		if e != nil || !r.IsValid() {
			invalid = true
			return
		}
		if !seen[r.Number] {
			seen[r.Number] = true
			results = append(results, r)
		}
	})
	if invalid {
		return nil, provider.ErrIncompleteMetadata
	}
	// 非空容器中没有可识别卡片意味着模板变化，不能视为无结果。
	if len(results) == 0 && (doc.Find(container).Children().Length() > 0 || text(doc.Find(container)) != "") {
		return nil, fmt.Errorf("%s: unrecognized search cards", Name)
	}
	return results, nil
}
func labelValue(s, key string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), key)), ":："))
}
func init() { provider.Register(Name, New) }
