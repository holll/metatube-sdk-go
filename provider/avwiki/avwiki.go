// Package avwiki 提供独立的 AV-Wiki 影片资料及演员识别结果。
package avwiki

import (
	"bytes"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

const (
	Name     = "AVWiki"
	Priority = 350
	baseURL  = "https://av-wiki.net/"
)

// ErrChallenge 区分验证页（包括 HTTP 200 响应）与正常空结果。
// 本来源不会尝试解决或绕过验证。
var ErrChallenge = errors.New("AVWiki: request verification required")

var (
	_             provider.MovieProvider = (*AVWiki)(nil)
	_             provider.MovieSearcher = (*AVWiki)(nil)
	slugPattern                          = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)
	numberPattern                        = regexp.MustCompile(`(?i)^[a-z0-9]*[a-z][a-z0-9]*[-_][0-9][a-z0-9_-]*$`)
)

type AVWiki struct{ *scraper.Scraper }

func New() *AVWiki {
	p := &AVWiki{scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese)}
	// 共享 HTTP 客户端只在初始化时配置，避免并发查询期间修改。
	p.ClonedCollector().SetRedirectHandler(func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return provider.ErrInvalidURL
		}
		if len(via) > 0 && validSearchURL(via[0].URL) {
			if !validSearchURL(req.URL) {
				return provider.ErrInvalidURL
			}
			return nil
		}
		_, err := p.ParseMovieIDFromURL(req.URL.String())
		return err
	})
	return p
}

func validSearchURL(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, "av-wiki.net") && u.User == nil && u.Path == "/" && u.Fragment == "" && u.Query().Has("s")
}

// ID 是单段小写 canonical slug，而非路径；数字前导零必须保留。
func (p *AVWiki) NormalizeMovieID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if !slugPattern.MatchString(id) || !strings.ContainsAny(id, "abcdefghijklmnopqrstuvwxyz") || !strings.ContainsAny(id, "0123456789") {
		return ""
	}
	return id
}

func (p *AVWiki) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, "av-wiki.net") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || strings.Contains(u.EscapedPath(), "%") {
		return "", provider.ErrInvalidURL
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, "/")
	id := p.NormalizeMovieID(path)
	if id == "" || strings.Contains(path, "/") || !strings.HasPrefix(u.Path, "/") {
		return "", provider.ErrInvalidURL
	}
	return id, nil
}

func compact(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// 无分隔符的 CID 输入必须搜索，不能猜测 CID 对应的详情路径。
func (p *AVWiki) GetMovieInfoByID(raw string) (*model.MovieInfo, error) {
	id := p.NormalizeMovieID(raw)
	if id == "" {
		return nil, provider.ErrInvalidID
	}
	matches := func(info *model.MovieInfo, cid string) bool {
		return compact(id) == compact(info.Number) || (cid != "" && compact(id) == compact(cid))
	}
	if strings.Contains(id, "-") {
		info, cid, err := p.movie(baseURL + id + "/")
		if err == nil {
			if info.ID == id || matches(info, cid) {
				return info, nil
			}
			return nil, provider.ErrInfoNotFound
		}
		if !errors.Is(err, provider.ErrInfoNotFound) {
			return nil, err
		}
	}
	results, err := p.SearchMovie(id)
	if err != nil {
		return nil, err
	}
	for _, result := range results {
		info, cid, err := p.movie(result.Homepage)
		if errors.Is(err, provider.ErrInfoNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.ID == id || matches(info, cid) {
			return info, nil
		}
	}
	return nil, provider.ErrInfoNotFound
}

func (p *AVWiki) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	info, _, err := p.movie(raw)
	return info, err
}

// document 使用共享 Scraper 后端，保留已配置的代理和超时设置。
func (p *AVWiki) document(raw string, search bool) (*goquery.Document, *url.URL, error) {
	c := p.ClonedCollector()
	valid := func(u *url.URL) bool {
		if search {
			return validSearchURL(u)
		}
		_, err := p.ParseMovieIDFromURL(u.String())
		return err == nil
	}
	var doc *goquery.Document
	var finalURL *url.URL
	var parseErr error
	challenge := func(body []byte) bool {
		text := strings.ToLower(string(body))
		return strings.Contains(text, "one moment, please") || strings.Contains(text, "request is being verified") || strings.Contains(text, "cf-chl-") || strings.Contains(text, "/cdn-cgi/challenge-platform/")
	}
	c.OnResponse(func(r *colly.Response) {
		if challenge(r.Body) {
			parseErr = ErrChallenge
			return
		}
		if !valid(r.Request.URL) {
			parseErr = provider.ErrInvalidURL
			return
		}
		finalURL = r.Request.URL
		doc, parseErr = goquery.NewDocumentFromReader(bytes.NewReader(r.Body))
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
		return nil, nil, parseErr
	}
	if err != nil {
		return nil, nil, err
	}
	if doc == nil {
		return nil, nil, provider.ErrIncompleteMetadata
	}
	return doc, finalURL, nil
}

func (p *AVWiki) movie(raw string) (*model.MovieInfo, string, error) {
	id, err := p.ParseMovieIDFromURL(raw)
	if err != nil {
		return nil, "", err
	}
	doc, finalURL, err := p.document(raw, false)
	if err != nil {
		return nil, "", err
	}
	id, err = p.ParseMovieIDFromURL(finalURL.String())
	if err != nil {
		return nil, "", err
	}
	homepage := finalURL.String()
	if canonical, ok := doc.Find(`link[rel="canonical"]`).First().Attr("href"); ok {
		u, e := url.Parse(strings.TrimSpace(canonical))
		if e != nil {
			return nil, "", provider.ErrInvalidURL
		}
		canonical = finalURL.ResolveReference(u).String()
		id, err = p.ParseMovieIDFromURL(canonical)
		if err != nil {
			return nil, "", err
		}
		homepage = canonical
	}
	info := &model.MovieInfo{ID: id, Provider: Name, Homepage: homepage, Actors: []string{}, PreviewImages: []string{}, Genres: []string{}}
	// 所有字段只取同一作品表格，不解析推荐表格、认人问答或标签链接。
	article := doc.Find("article.article").First()
	table := article.ChildrenFiltered(".article-body").Find("dl.dltable").First()
	values := map[string]*goquery.Selection{}
	table.ChildrenFiltered("dt").Each(func(_ int, dt *goquery.Selection) {
		dd := dt.Next()
		if goquery.NodeName(dd) == "dd" {
			key := strings.TrimSpace(dt.Text())
			if values[key] == nil {
				values[key] = dd
			}
		}
	})
	text := func(key string) string {
		if s := values[key]; s != nil {
			return strings.TrimSpace(s.Text())
		}
		return ""
	}
	info.Number = text("メーカー品番")
	cid := text("FANZA品番")
	info.Label = text("レーベル")
	info.ReleaseDate = parser.ParseDate(text("配信開始日"))
	if actors := values["AV女優名"]; actors != nil {
		info.Actors = parseActors(actors)
	}
	// 主图位于当前文章头部，不从推荐作品网格取图。
	images := article.ChildrenFiltered("header").Find(".article-thumbnail img")
	var imageTitle string
	images.Each(func(_ int, img *goquery.Selection) {
		if info.CoverURL != "" {
			return
		}
		if image := imageURL(img, finalURL); image != "" {
			info.CoverURL = image
			imageTitle = strings.TrimSpace(img.AttrOr("alt", ""))
		}
	})
	// 仅考虑当前文章中明确标记的 product 区块；
	// 即使推荐容器包含 product，imageURL 也会排除其中的图片。
	article.Find(".article-body .product img").Each(func(_ int, img *goquery.Selection) {
		if info.CoverURL == "" {
			info.CoverURL = imageURL(img, finalURL)
		}
	})
	info.Title = cleanTitle(imageTitle, info.Label, info.Number)
	if info.Title == "" {
		info.Title = cleanTitle(text("作品タイトル"), info.Label, info.Number)
	}
	info.ThumbURL = info.CoverURL
	info.BigCoverURL = info.CoverURL
	info.BigThumbURL = info.CoverURL
	if !info.IsValid() {
		return nil, "", provider.ErrIncompleteMetadata
	}
	return info, cid, nil
}

func parseActors(dd *goquery.Selection) []string {
	clone := dd.Clone()
	clone.Find("br").ReplaceWithHtml("\n")
	clone.Find("a").Each(func(_ int, a *goquery.Selection) { a.ReplaceWithHtml("\n" + html.EscapeString(a.Text()) + "\n") })
	result := []string{}
	seen := map[string]bool{}
	for _, name := range strings.FieldsFunc(clone.Text(), func(r rune) bool {
		return r == '\n' || r == '\r' || r == '、' || r == '，' || r == ',' || r == '/' || r == '／' || r == ';' || r == '；'
	}) {
		name = strings.TrimSpace(name)
		switch strings.ToLower(name) {
		case "", "(≥o≤)", "（≥o≤）", "不明", "不詳", "不明です", "不明（調査中）", "調査中", "未確認", "unknown", "n/a", "なし", "-", "―", "？", "?":
			continue
		}
		if strings.Trim(name, "*＊ ?？-―　") == "" || seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

func cleanTitle(title, label, number string) string {
	title = strings.TrimSpace(title)
	if title == "誰？" || title == "誰?" || strings.Contains(title, "AV女優は誰") || strings.Contains(title, "名前は？") || strings.Contains(title, "名前は?") {
		return ""
	}
	if label != "" && number != "" {
		for _, dash := range []string{" - ", " – ", " — ", " − "} {
			decoration := label + dash + number
			for _, suffix := range []string{"（" + decoration + "）", "(" + decoration + ")"} {
				title = strings.TrimSpace(strings.TrimSuffix(title, suffix))
			}
		}
	}
	return title
}

func imageURL(img *goquery.Selection, base *url.URL) string {
	if img.ParentsFiltered(".col_4, .related, .related-posts, .recommend, .recommendations").Length() > 0 {
		return ""
	}
	for _, attr := range []string{"data-src", "src"} {
		raw := strings.TrimSpace(img.AttrOr(attr, ""))
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		u = base.ResolveReference(u)
		path := strings.ToLower(u.Path)
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".ico") || strings.Contains(path, "logo") || strings.Contains(path, "icon") || strings.Contains(path, "placeholder") {
			continue
		}
		return u.String()
	}
	return ""
}

func (p *AVWiki) NormalizeMovieKeyword(keyword string) string { return strings.TrimSpace(keyword) }

func (p *AVWiki) SearchMovie(keyword string) ([]*model.MovieSearchResult, error) {
	keyword = p.NormalizeMovieKeyword(keyword)
	if keyword == "" {
		return nil, provider.ErrInvalidKeyword
	}
	doc, base, err := p.document(baseURL+"?"+url.Values{"s": {keyword}}.Encode(), true)
	if err != nil {
		return nil, err
	}
	results := []*model.MovieSearchResult{}
	seen := map[string]bool{}
	cards := doc.Find("article.archive-list")
	// 无卡片时只接受 main 中明确的日文无结果提示，未知模板不能伪装成空结果。
	if cards.Length() == 0 {
		if strings.TrimSpace(doc.Find("main").First().Text()) == "見つかりませんでした" {
			return results, nil
		}
		return nil, provider.ErrIncompleteMetadata
	}
	incomplete := false
	cards.Each(func(_ int, card *goquery.Selection) {
		var id, homepage string
		links := card.Find(".read-more a[href]")
		if links.Length() == 0 {
			incomplete = true
			return
		}
		links.Each(func(_ int, a *goquery.Selection) {
			if id != "" {
				return
			}
			href := strings.TrimSpace(a.AttrOr("href", ""))
			u, e := url.Parse(href)
			if e != nil || href == "" {
				incomplete = true
				return
			}
			candidate := base.ResolveReference(u).String()
			if slug, e := p.ParseMovieIDFromURL(candidate); e == nil {
				id = slug
				homepage = candidate
			}
		})
		if id == "" || seen[id] {
			return
		}
		title := strings.TrimSpace(card.Find(".archive-header-title").First().Text())
		if title == "" {
			title = strings.TrimSpace(card.Find(".eye-catch img").First().AttrOr("alt", ""))
		}
		if title == "" {
			incomplete = true
			return
		}
		number := ""
		card.Find(".post-meta li").Each(func(_ int, li *goquery.Selection) {
			s := strings.TrimSpace(li.Text())
			if number == "" && numberPattern.MatchString(s) {
				number = s
			}
		})
		if number == "" {
			incomplete = true
			return
		}
		image := imageURL(card.Find(".eye-catch img").First(), base)
		seen[id] = true
		results = append(results, &model.MovieSearchResult{ID: id, Number: number, Title: title, Provider: Name, Homepage: homepage, ThumbURL: image, CoverURL: image})
	})
	if incomplete {
		return nil, provider.ErrIncompleteMetadata
	}
	return results, nil
}

func init() { provider.Register(Name, New) }
