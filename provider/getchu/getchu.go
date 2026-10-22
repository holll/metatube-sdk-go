package getchu

import (
	"bytes"
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

var _ provider.MovieProvider = (*Getchu)(nil)

const (
	Name     = "Getchu"
	Priority = 1000
	baseURL  = "https://dl.getchu.com/"
)

var (
	ErrChallenge   = errors.New("Getchu: request verification required")
	idPattern      = regexp.MustCompile(`^[1-9][0-9]*$`)
	inputPattern   = regexp.MustCompile(`(?i)^(?:GETCHU[-_])?([1-9][0-9]*)$`)
	runtimePattern = regexp.MustCompile(`(?:動画[：:]\s*|収録分数\s*約?\s*)([0-9]+)\s*分`)
)

type Getchu struct{ *scraper.Scraper }

func New() *Getchu {
	p := &Getchu{scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese, scraper.WithDetectCharset())}
	p.ClonedCollector().SetRedirectHandler(func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return provider.ErrInvalidURL
		}
		_, err := p.ParseMovieIDFromURL(r.URL.String())
		return err
	})
	return p
}

func (gcu *Getchu) NormalizeMovieID(id string) string {
	if ss := inputPattern.FindStringSubmatch(strings.TrimSpace(id)); len(ss) == 2 {
		return ss[1]
	}
	return ""
}

func (gcu *Getchu) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	id = gcu.NormalizeMovieID(id)
	if id == "" {
		return nil, provider.ErrInvalidID
	}
	return gcu.GetMovieInfoByURL(baseURL + "i/item" + id)
}

func (gcu *Getchu) ParseMovieIDFromURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !strings.EqualFold(u.Host, "dl.getchu.com") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || strings.Contains(u.EscapedPath(), "%") {
		return "", provider.ErrInvalidURL
	}
	id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/i/item"), "/")
	if !idPattern.MatchString(id) || (u.Path != "/i/item"+id && u.Path != "/i/item"+id+"/") {
		return "", provider.ErrInvalidURL
	}
	return id, nil
}

func challenge(doc *goquery.Document) bool {
	title := strings.ToLower(strings.TrimSpace(doc.Find("title").Text()))
	return doc.Find("#challenge-form, #cf-challenge-running, .g-recaptcha, #captcha").Length() > 0 || strings.Contains(title, "just a moment") || strings.Contains(title, "captcha") || strings.Contains(title, "access denied") || strings.Contains(title, "年齢認証") || strings.Contains(title, "年齢確認")
}

func (gcu *Getchu) GetMovieInfoByURL(raw string) (*model.MovieInfo, error) {
	requestedID, err := gcu.ParseMovieIDFromURL(raw)
	if err != nil {
		return nil, err
	}
	c := gcu.ClonedCollector()
	var doc *goquery.Document
	var final *url.URL
	var responseErr error
	c.OnResponse(func(r *colly.Response) {
		final = r.Request.URL
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
		return nil, responseErr
	}
	if visitErr != nil {
		return nil, visitErr
	}
	if doc == nil || final == nil {
		return nil, provider.ErrIncompleteMetadata
	}
	id, err := gcu.ParseMovieIDFromURL(final.String())
	if err != nil {
		return nil, err
	}
	if id != requestedID {
		return nil, provider.ErrInfoNotFound
	}
	if canonical, ok := doc.Find(`meta[property="og:url"]`).First().Attr("content"); ok {
		u, e := url.Parse(strings.TrimSpace(canonical))
		if e != nil {
			return nil, provider.ErrInvalidURL
		}
		canonicalID, e := gcu.ParseMovieIDFromURL(final.ResolveReference(u).String())
		if e != nil {
			return nil, e
		}
		if canonicalID != id {
			return nil, provider.ErrInfoNotFound
		}
	}
	info := &model.MovieInfo{ID: id, Number: "GETCHU-" + id, Provider: Name, Homepage: baseURL + "i/item" + id, Actors: []string{}, Genres: []string{}, PreviewImages: []string{}}
	info.Title = strings.TrimSpace(doc.Find(`meta[property="og:title"]`).First().AttrOr("content", ""))
	if info.Title == "" {
		info.Title = strings.TrimSpace(doc.Find(`div[style="color: #333333; padding: 3px 0px 0px 5px;"]`).First().Text())
	}
	info.CoverURL = mediaURL(doc.Find(`meta[property="og:image"]`).First().AttrOr("content", ""), final)
	if info.CoverURL == "" {
		doc.Find("img[src]").EachWithBreak(func(_ int, img *goquery.Selection) bool {
			u := mediaURL(img.AttrOr("src", ""), final)
			if strings.Contains(u, "/"+id+"/"+id+"top.") {
				info.CoverURL = u
				return false
			}
			return true
		})
	}
	var fields *goquery.Selection
	doc.Find("td.bluetext").EachWithBreak(func(_ int, td *goquery.Selection) bool {
		if strings.TrimSpace(td.Text()) == "サークル" {
			fields = td.Closest("table")
			return false
		}
		return true
	})
	if fields == nil {
		return nil, provider.ErrIncompleteMetadata
	}
	fields.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		cells := tr.ChildrenFiltered("td")
		if cells.Length() != 2 {
			return
		}
		v := cells.Eq(1)
		s := strings.TrimSpace(v.Text())
		switch strings.TrimSpace(cells.First().Text()) {
		case "サークル":
			info.Label = s
		case "配信開始日":
			info.ReleaseDate = parser.ParseDate(s)
		case "画像数&ページ数":
			info.Runtime = parser.ParseRuntime(runtimePattern.FindString(s))
		case "趣向":
			v.Find("a").Each(func(_ int, a *goquery.Selection) {
				info.Genres = appendUnique(info.Genres, strings.TrimSpace(a.Text()))
			})
		case "作品内容":
			info.Summary = summary(v)
		}
	})
	doc.Find(`td[style*="background-color: #444444;"] a[href]`).Each(func(_ int, a *goquery.Selection) {
		u := mediaURL(a.AttrOr("href", ""), final)
		if strings.Contains(u, "/"+id+"/"+id+"_") {
			info.PreviewImages = appendUnique(info.PreviewImages, u)
		}
	})
	info.ThumbURL, info.BigCoverURL, info.BigThumbURL = info.CoverURL, info.CoverURL, info.CoverURL
	if !info.IsValid() {
		return nil, provider.ErrIncompleteMetadata
	}
	return info, nil
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
		if s == v {
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
