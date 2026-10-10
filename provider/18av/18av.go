package eighteenav

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gocolly/colly/v2"
	"golang.org/x/text/language"

	"github.com/metatube-community/metatube-sdk-go/common/parser"
	"github.com/metatube-community/metatube-sdk-go/model"
	"github.com/metatube-community/metatube-sdk-go/provider"
	"github.com/metatube-community/metatube-sdk-go/provider/internal/scraper"
)

var (
	_ provider.MovieProvider = (*EighteenAV)(nil)
	_ provider.MovieSearcher = (*EighteenAV)(nil)
)

const (
	Name     = "18AV"
	Priority = 500
)

// 站点提供 zh/en/ja/ko 多语言版本。这里固定使用日语页：
// 繁中页会翻译演员名与类型名（如「女子校生」被译作「高中女生」），
// 日语页保留原始日文写法，元数据更准确。
const (
	baseURL   = "https://18av.mm-cg.com/ja/"
	searchURL = baseURL + "fc_search/all/%s/1.html" // fc = 全部影片
)

// slugRe 匹配影片页 URL 末段中可用的标识，如 "NHDTB-817"、"fc2-ppv-4878910"。
var slugRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// coverPathRe 捕获封面 URL 的 "/b/{category}/{name}.jpg" 部分。
var coverPathRe = regexp.MustCompile(`^/b/([^/]+)/(.+)\.jpg$`)

// emptySlug 是部分分类（如 dt_content）固定使用的占位末段，不含影片身份信息。
const emptySlug = "content"

// unknownValues 是站点用来表示「无此信息」的占位值，统一归一为空串，
// 避免 "----" / "N/A" 这类字面量混进元数据。
var unknownValues = map[string]bool{
	"----": true,
	"-":    true,
	"n/a":  true,
	"素人":   true, // 出演者占位（配图为 nowprinting.gif，并非演员名）
}

func cleanValue(s string) string {
	s = strings.TrimSpace(s)
	if unknownValues[strings.ToLower(s)] {
		return ""
	}
	return s
}

type EighteenAV struct {
	*scraper.Scraper
}

func New() *EighteenAV {
	return &EighteenAV{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}

func (av *EighteenAV) NormalizeMovieID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}

// ParseMovieIDFromURL 从详情页 URL 中解析影片 ID。
//
// URL 形如 /{lang}/{type}_content/{internalID}/{slug}.html。
// slug 通常就是品番（"NHDTB-817"），可直接用于 GetMovieInfoByID 反查；
// 但少数分类（如 dt_content）的 slug 固定为 "content"，不含身份信息，
// 此时回退为内部数字 ID（路径倒数第二段）。
//
// 注意：内部数字 ID 由站点数据库自增生成，与品番无推导关系，
// 因此无法由 ID 直接拼出详情页 URL，GetMovieInfoByID 必须走搜索。
func (av *EighteenAV) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	if slug := strings.TrimSuffix(segs[len(segs)-1], ".html"); slug != emptySlug && slugRe.MatchString(slug) {
		return slug, nil
	}
	return segs[len(segs)-2], nil
}

// GetMovieInfoByID 通过搜索定位详情页：详情页 URL 依赖内部数字 ID，
// 无法由品番拼出，只能先搜后取。
func (av *EighteenAV) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	results, err := av.SearchMovie(av.NormalizeMovieKeyword(id))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, provider.ErrInfoNotFound
	}
	// 优先取品番完全匹配的结果，避免同名系列串味。
	target := results[0]
	for _, r := range results {
		if strings.EqualFold(r.Number, id) {
			target = r
			break
		}
	}
	return av.GetMovieInfoByURL(target.Homepage)
}

func (av *EighteenAV) GetMovieInfoByURL(rawURL string) (info *model.MovieInfo, err error) {
	id, err := av.ParseMovieIDFromURL(rawURL)
	if err != nil {
		return
	}

	info = &model.MovieInfo{
		ID:            id,
		Provider:      av.Name(),
		Homepage:      rawURL,
		Actors:        []string{},
		PreviewImages: []string{},
		Genres:        []string{},
	}

	// ld+json 提供语言无关的结构化数据，作为主标题/日期/封面的兜底来源。
	var ldName, ldThumb, ldDate string
	// HTML 各来源先各自收集，最后在 OnScraped 中按优先级组装，避免依赖回调顺序。
	var h1Title, coverURL, releaseDate string

	c := av.ClonedCollector()

	c.OnHTML(`script[type="application/ld+json"]`, func(e *colly.HTMLElement) {
		var ld struct {
			Name         string `json:"name"`
			ThumbnailURL string `json:"thumbnailUrl"`
			UploadDate   string `json:"uploadDate"`
		}
		if json.Unmarshal([]byte(e.Text), &ld) != nil {
			return
		}
		ldName, ldThumb, ldDate = ld.Name, ld.ThumbnailURL, ld.UploadDate
	})

	// 标题：形如 <h1><b>NHDTB-817 ロングスカート女学生…</b></h1>
	c.OnXML(`//div[@class='archive-title']/h1`, func(e *colly.XMLElement) {
		if h1Title == "" {
			h1Title = strings.TrimSpace(e.Text)
		}
	})

	// 封面：播放器占位图上挂着大图，普通影片与 dt 分类都适用。
	c.OnXML(`//div[@id='player-wrap']/img`, func(e *colly.XMLElement) {
		if coverURL == "" {
			coverURL = e.Request.AbsoluteURL(e.Attr("src"))
		}
	})

	// 信息块为 <li class='posts-headline'>品番:</li><li class='posts-message'>NHDTB-817</li> 结构。
	field := func(label string) string {
		return fmt.Sprintf(`//li[@class='posts-headline' and starts-with(normalize-space(.), '%s')]`+
			`/following-sibling::li[@class='posts-message'][1]`, label)
	}

	c.OnXML(field("品番"), func(e *colly.XMLElement) {
		if info.Number == "" {
			info.Number = cleanValue(e.Text)
		}
	})

	c.OnXML(field("収録時間"), func(e *colly.XMLElement) {
		if info.Runtime == 0 {
			info.Runtime = parser.ParseRuntime(e.Text)
		}
	})

	c.OnXML(field("監督"), func(e *colly.XMLElement) {
		if info.Director == "" {
			info.Director = cleanValue(e.Text)
		}
	})

	c.OnXML(field("レーベル"), func(e *colly.XMLElement) {
		if info.Label == "" {
			info.Label = cleanValue(e.Text)
		}
	})

	c.OnXML(field("メーカー"), func(e *colly.XMLElement) {
		if info.Maker == "" {
			info.Maker = cleanValue(e.Text)
		}
	})

	// 发布日期标签在日语页上仍写作「網站發佈」。
	c.OnXML(field("網站發佈"), func(e *colly.XMLElement) {
		if releaseDate == "" {
			releaseDate = cleanValue(e.Text)
		}
	})

	c.OnXML(`//span[@class='posts-inner-details-text-under']//a`, func(e *colly.XMLElement) {
		if genre := cleanValue(e.Text); genre != "" {
			info.Genres = append(info.Genres, genre)
		}
	})

	c.OnXML(`//div[@class='actor-right-images-part']/p`, func(e *colly.XMLElement) {
		if name := cleanValue(e.Text); name != "" {
			info.Actors = append(info.Actors, name)
		}
	})

	c.OnScraped(func(_ *colly.Response) {
		info.Title = firstNonEmpty(ldName, trimNumberPrefix(h1Title, info.Number))
		if info.Title == "" {
			err = provider.ErrInfoNotFound
			return
		}

		// 品番缺失时（dt 等无编号分类）由 ID 推导。
		if info.Number == "" {
			info.Number = parser.ParseIDToNumber(info.ID)
		}

		info.CoverURL = firstNonEmpty(coverURL, ldThumb)
		info.BigCoverURL = info.CoverURL
		info.ThumbURL, info.PreviewVideoURL = buildMediaURLs(info.CoverURL)
		info.BigThumbURL = info.ThumbURL

		info.ReleaseDate = parser.ParseDate(firstNonEmpty(releaseDate, ldDate))
	})

	err = c.Visit(info.Homepage)
	return
}

func (av *EighteenAV) NormalizeMovieKeyword(keyword string) string {
	return strings.ToUpper(strings.TrimSpace(keyword))
}

func (av *EighteenAV) SearchMovie(keyword string) (results []*model.MovieSearchResult, err error) {
	c := av.ClonedCollector()

	// 结果条目形如：
	//   <div class='post video_9s'>
	//     <figure class='video-preview thumb'><a href="…/{id}/{slug}.html"><img src='…/s/…'></a></figure>
	//     <div class='con'>
	//       <h3><a href="…">{品番} {标题}</a></h3>
	//       <div class='meta'>2023/09/25</div>
	//     </div>
	//   </div>
	c.OnXML(`//div[@class='post video_9s']`, func(e *colly.XMLElement) {
		href := e.ChildAttr(`.//h3/a`, "href")
		if href == "" {
			return
		}
		homepage := e.Request.AbsoluteURL(href)
		id, idErr := av.ParseMovieIDFromURL(homepage)
		if idErr != nil {
			return
		}

		thumbURL := e.Request.AbsoluteURL(e.ChildAttr(`.//figure//img`, "src"))

		results = append(results, &model.MovieSearchResult{
			ID:          id,
			Number:      parser.ParseIDToNumber(id),
			Title:       strings.TrimSpace(e.ChildText(`.//h3/a`)),
			Provider:    av.Name(),
			Homepage:    homepage,
			ThumbURL:    thumbURL,
			CoverURL:    strings.Replace(thumbURL, "/s/", "/b/", 1),
			ReleaseDate: parser.ParseDate(e.ChildText(`.//div[@class='con']/div[@class='meta']`)),
		})
	})

	// keyword 是路径段而非查询参数，必须用 PathEscape（QueryEscape 会把空格转成 '+'）。
	err = c.Visit(fmt.Sprintf(searchURL, url.PathEscape(keyword)))
	return
}

// buildMediaURLs 由封面 URL 推导缩略图与预览片地址。
//
// 封面形如 https://fchost1.imgstream2.com/b/censored/334998_NHDTB-817.jpg，
// 对应缩略图为同路径 /b/ → /s/，预览片则换到 gifb 主机并加 /hc/9smv_ 前缀。
// 图床域名会轮换（imgstream2/3…）且 fbhost1/fchost1 随机，故一律从封面 URL 现场推导，
// 不硬编码主机名。路径不符合预期时返回空串，由调用方留空即可。
func buildMediaURLs(coverURL string) (thumb, preview string) {
	u, err := url.Parse(coverURL)
	if err != nil {
		return "", ""
	}
	m := coverPathRe.FindStringSubmatch(u.Path)
	if len(m) != 3 {
		return "", ""
	}
	category, name := m[1], m[2]

	thumbURL := *u
	thumbURL.Path = "/s/" + category + "/" + name + ".jpg"
	thumb = thumbURL.String()

	// "fchost1.imgstream2.com" 去掉首段即图床基础域名。
	parts := strings.SplitN(u.Host, ".", 2)
	if len(parts) != 2 {
		return thumb, ""
	}
	return thumb, fmt.Sprintf("https://gifb.%s/hc/9smv_%s_%s.mp4", parts[1], category, name)
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// trimNumberPrefix 去掉标题开头的品番前缀，如
// ("NHDTB-817 ロングスカート…", "NHDTB-817") -> "ロングスカート…"。
// 仅当品番之后紧跟非字母数字字符时才视为前缀，否则 "NHDTB-8179" 会被 "NHDTB-817" 误截。
func trimNumberPrefix(title, number string) string {
	if number == "" || len(title) < len(number) ||
		!strings.EqualFold(title[:len(number)], number) {
		return title
	}
	rest := title[len(number):]
	if r, _ := utf8.DecodeRuneInString(rest); unicode.IsLetter(r) || unicode.IsDigit(r) {
		return title
	}
	return strings.TrimSpace(rest)
}

func init() {
	provider.Register(Name, New)
}
