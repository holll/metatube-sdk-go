// Package airavcity 提供 airavcity4.cc（瘋AV / airav.io）的影片资料。
package airavcity

import (
	"errors"
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
	Name     = "AirAVCity"
	Priority = 300
	baseURL  = "https://airavcity4.cc/"
)

// ErrChallenge 区分 Cloudflare 挑战页与正常空结果。
// 本来源不会尝试解决或绕过验证。
var ErrChallenge = errors.New("AirAVCity: request verification required")

var (
	_ provider.MovieProvider = (*AirAVCity)(nil)
	_ provider.MovieSearcher = (*AirAVCity)(nil)

	// hid 形如 99-21-14639、QC-BT-4734904、127999。
	hidPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	// 标准番号形如 STARS-087、FNS-266；hid 也可能含横杠，靠字母前缀区分。
	numberPattern = regexp.MustCompile(`^[a-z][a-z0-9]*-[0-9]+$`)
	// 搜索卡片标题以番号开头，用于提取单部作品的番号。
	titlePrefix = regexp.MustCompile(`^([A-Za-z0-9]+(?:-[A-Za-z0-9]+)*)\s+`)
)

type AirAVCity struct{ *scraper.Scraper }

func New() *AirAVCity {
	p := &AirAVCity{scraper.NewDefaultScraper(Name, baseURL, Priority, language.Chinese)}
	// 共享 HTTP 客户端只在初始化时配置，避免并发查询期间修改。
	p.ClonedCollector().SetRedirectHandler(func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return provider.ErrInvalidURL
		}
		// 只有搜索页发起的跳转按搜索规则校验，其余按详情规则。
		search := len(via) > 0 && via[0].URL.Path == "/search_result"
		return p.validateRequestURL(req.URL, search)
	})
	return p
}

func (p *AirAVCity) validateRequestURL(u *url.URL, search bool) error {
	if (u.Scheme != "http" && u.Scheme != "https") ||
		!strings.EqualFold(u.Host, "airavcity4.cc") ||
		u.User != nil || u.Port() != "" || u.Fragment != "" || u.RawFragment != "" {
		return provider.ErrInvalidURL
	}
	// 详情与搜索是两类页面，跳转不能把一类换成另一类。
	if search {
		if u.Path != "/search_result" || u.Query().Get("kw") == "" {
			return provider.ErrInvalidURL
		}
		return nil
	}
	if u.Path != "/video" {
		return provider.ErrInvalidURL
	}
	_, err := p.ParseMovieIDFromURL(u.String())
	return err
}

// ID 是站内 hid，与番号无对应关系，因此不能由番号推导。
func (p *AirAVCity) NormalizeMovieID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if !hidPattern.MatchString(id) || !strings.ContainsAny(id, "0123456789") {
		return ""
	}
	return id
}

func (p *AirAVCity) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
		!strings.EqualFold(u.Host, "airavcity4.cc") || u.User != nil || u.Port() != "" ||
		u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" ||
		u.Path != "/video" || strings.Contains(u.EscapedPath(), "%") {
		return "", provider.ErrInvalidURL
	}
	query := u.Query()
	if len(query) != 1 {
		return "", provider.ErrInvalidURL
	}
	id := p.NormalizeMovieID(query.Get("hid"))
	if id == "" {
		return "", provider.ErrInvalidURL
	}
	return id, nil
}

func isStandardNumber(id string) bool { return numberPattern.MatchString(id) }

func (p *AirAVCity) GetMovieInfoByID(raw string) (*model.MovieInfo, error) {
	id := p.NormalizeMovieID(raw)
	if id == "" {
		return nil, provider.ErrInvalidID
	}
	// 标准番号必须搜索回查 hid；hid 本身直接取详情。
	if !isStandardNumber(id) {
		return p.GetMovieInfoByURL(baseURL + "video?hid=" + url.QueryEscape(id))
	}
	results, err := p.SearchMovie(id)
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		if !strings.EqualFold(result.Number, id) {
			continue
		}
		info, err := p.GetMovieInfoByURL(result.Homepage)
		if err != nil {
			return nil, err
		}
		if !strings.EqualFold(info.Number, id) {
			return nil, provider.ErrInfoNotFound
		}
		return info, nil
	}
	return nil, provider.ErrInfoNotFound
}

func (p *AirAVCity) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	id, err := p.ParseMovieIDFromURL(raw)
	if err != nil {
		return nil, err
	}
	doc, err := p.document(raw, false)
	if err != nil {
		return nil, err
	}
	return p.parseDetail(doc, id)
}

func (p *AirAVCity) parseDetail(doc *goquery.Document, id string) (*model.MovieInfo, error) {
	info := &model.MovieInfo{
		ID:            id,
		Provider:      Name,
		Homepage:      baseURL + "video?hid=" + url.QueryEscape(id),
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}
	info.Title = strings.TrimSpace(doc.Find(".video-title h1").First().Text())
	// 发布日期位于播放器信息条，格式为 2019-06-20 00:00:00。
	if clock := doc.Find(".video-item .fa-clock").First().Parent(); clock.Length() > 0 {
		info.ReleaseDate = parser.ParseDate(strings.TrimSpace(clock.Text()))
	}
	info.Summary = strings.TrimSpace(doc.Find(".video-info > p").First().Text())

	doc.Find(".info-list li").Each(func(_ int, li *goquery.Selection) {
		label, value := splitField(li)
		switch label {
		case "番號":
			info.Number = value
		case "女優":
			info.Actors = collect(li, `a[href^="/actor?id="]`, info.Actors)
		case "標籤":
			info.Genres = collect(li, `a[href^="/tag?tid="]`, info.Genres)
		case "廠商":
			if maker := strings.TrimSpace(li.Find(`a[href^="/tag?fid="]`).First().Text()); maker != "" {
				info.Maker = maker
			}
		}
	})

	// 推荐作品同样使用 /storage/cover/big/ 路径，只能取 og:image。
	if cover, ok := doc.Find(`meta[property="og:image"]`).First().Attr("content"); ok {
		if u := strings.TrimSpace(cover); strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			info.CoverURL = u
		}
	}
	// 站点标题以番号开头，与搜索结果保持一致时去掉该前缀。
	if match := titlePrefix.FindStringSubmatch(info.Title); len(match) == 2 &&
		strings.EqualFold(match[1], info.Number) {
		info.Title = strings.TrimSpace(strings.TrimPrefix(info.Title, match[0]))
	}
	if info.CoverURL != "" {
		info.ThumbURL = info.CoverURL
		info.BigCoverURL = info.CoverURL
		info.BigThumbURL = info.CoverURL
	}
	if !info.IsValid() {
		return nil, provider.ErrIncompleteMetadata
	}
	return info, nil
}

// splitField 按「標籤：值」把列表项拆成标签和纯文本值。
func splitField(li *goquery.Selection) (label, value string) {
	text := strings.TrimSpace(li.Text())
	idx := strings.IndexAny(text, "：:")
	if idx < 0 {
		return "", text
	}
	// 全角冒号占 3 字节，半角占 1 字节。
	sep := len("：")
	if text[idx] == ':' {
		sep = 1
	}
	return strings.TrimSpace(text[:idx]), strings.TrimSpace(text[idx+sep:])
}

func collect(li *goquery.Selection, selector string, dst []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, v := range dst {
		seen[v] = true
	}
	li.Find(selector).Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		dst = append(dst, name)
	})
	return dst
}

func (p *AirAVCity) NormalizeMovieKeyword(keyword string) string {
	return strings.Join(strings.Fields(keyword), " ")
}

func (p *AirAVCity) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	keyword = p.NormalizeMovieKeyword(keyword)
	if keyword == "" {
		return nil, provider.ErrInvalidKeyword
	}
	doc, err := p.document(baseURL+"search_result?kw="+url.QueryEscape(keyword), true)
	if err != nil {
		return nil, err
	}
	cards := doc.Find("div.row div.col.oneVideo")
	// 无卡片时只接受结构完整的搜索页，未知模板不能伪装成空结果。
	if cards.Length() == 0 {
		if doc.Find("div.row.row-cols-2").Length() == 0 {
			return nil, provider.ErrIncompleteMetadata
		}
		return []*model.MovieSearchResult{}, nil
	}
	results := []*model.MovieSearchResult{}
	seen := map[string]bool{}
	incomplete := false
	cards.Each(func(_ int, card *goquery.Selection) {
		href, ok := card.Find(`a[href^="/video?hid="]`).First().Attr("href")
		if !ok {
			incomplete = true
			return
		}
		homepage := baseURL + strings.TrimPrefix(strings.TrimSpace(href), "/")
		id, e := p.ParseMovieIDFromURL(homepage)
		if e != nil {
			incomplete = true
			return
		}
		title := strings.TrimSpace(card.Find(".oneVideo-body h5").First().Text())
		if title == "" {
			// 有详情链接却没有标题，属于模板损坏，不能当成合集跳过。
			incomplete = true
			return
		}
		match := titlePrefix.FindStringSubmatch(title)
		if len(match) != 2 {
			// 系列合集等非单部作品没有番号前缀，跳过而不是判定为损坏。
			return
		}
		if seen[id] {
			return
		}
		seen[id] = true
		// 统一用规范化后的 hid 拼 Homepage，避免大小写混用导致重复请求。
		homepage = baseURL + "video?hid=" + url.QueryEscape(id)
		image := ""
		if src, ok := card.Find("img.card-img-top").First().Attr("src"); ok {
			if u := strings.TrimSpace(src); strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
				image = u
			}
		}
		results = append(results, &model.MovieSearchResult{
			ID:       id,
			Number:   strings.ToUpper(match[1]),
			Title:    strings.TrimSpace(strings.TrimPrefix(title, match[0])),
			Provider: Name,
			Homepage: homepage,
			ThumbURL: image,
			CoverURL: image,
		})
	})
	if incomplete {
		return nil, provider.ErrIncompleteMetadata
	}
	return results, nil
}

// document 使用共享 Scraper 后端，保留已配置的代理和超时设置。
func (p *AirAVCity) document(raw string, search bool) (*goquery.Document, error) {
	c := p.ClonedCollector()
	var doc *goquery.Document
	var parseErr error
	challenge := func(body []byte) bool {
		text := strings.ToLower(string(body))
		return strings.Contains(text, "just a moment") ||
			strings.Contains(text, "cf-chl-") ||
			strings.Contains(text, "/cdn-cgi/challenge-platform/")
	}
	c.OnResponse(func(r *colly.Response) {
		if challenge(r.Body) {
			parseErr = ErrChallenge
			return
		}
		if err := p.validateRequestURL(r.Request.URL, search); err != nil {
			parseErr = err
			return
		}
		doc, parseErr = goquery.NewDocumentFromReader(strings.NewReader(string(r.Body)))
	})
	c.OnError(func(r *colly.Response, err error) {
		switch {
		case r != nil && challenge(r.Body):
			parseErr = ErrChallenge
		case r != nil && r.StatusCode == http.StatusNotFound:
			parseErr = provider.ErrInfoNotFound
		default:
			parseErr = err
		}
	})
	err := c.Visit(raw)
	if parseErr != nil {
		return nil, parseErr
	}
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, provider.ErrIncompleteMetadata
	}
	return doc, nil
}

func init() { provider.Register(Name, New) }
