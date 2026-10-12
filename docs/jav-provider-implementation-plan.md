# JAV 聚合站 provider 落地方案(5 站)

调研与结构分析已完成,本文给出可直接照着写代码的实现方案。
前置调研见 [`jav-tube-sites-survey.md`](./jav-tube-sites-survey.md)。

## 零、优先级与结论(重要变更)

**实测发现数据质量问题,优先级相比初版调研有重大调整。**

同一部影片 SSIS-001(真实发行 2021-02)在 5 站的实测值:

| 站点 | 发行日期 | 标题 |
| --- | --- | --- |
| javtrailers | `2021-02-17` ✅ | `SSIS-001 一ヶ月間の禁欲の果てに… 葵つかさ 乙白さやか` 干净 |
| njavtv | `2021-02-21` ✅ | 同上,干净 |
| javhdporn | `2月 3, 2026` ❌ | `SSIS-001 [英語字幕] …` 带站点标注 |
| javxxx | `2024-10-17` ❌ | `SSIS-001 [英語字幕] …` 带站点标注 |
| bestjavporn | `2022/08/29` ❌ | `SSIS-001 [無修正流出] …` 带站点标注 |

三个 WordPress 站的「配信開始日 / 商品発売日」实际是**页面发布/修改日期**,不是影片发行日期,
误差可达数年;标题还带 `[英語字幕]` / `[無修正流出]` 这类站点自加的版本标注,并非原始标题。

另外,**javhdporn / javxxx / bestjavporn 是同一运营方**:都用 `pics.pornfhd.com` 图床、
同一套 WP 主题(相同 `img_onerror`、`data-video-uid`、`#video-actors` 结构),片库高度重叠。
**全接等于同一数据源接三遍,建议只取其一。**

### 最终优先级

| 优先级 | 站点 | Name | 包名 | 理由 | 代价 |
| --- | --- | --- | --- | --- | --- |
| **P0** | javtrailers.com | `JAVTRAILERS` | `javtrailers` | Nuxt SSR 页面轻(50KB)、有 ld+json `VideoObject`、日期正确、标题干净、演员/类型/监督/厂商齐全 | 番号无法直出 URL,须 search-then-fetch |
| **P1** | njavtv.com | `NJAVTV` | `njavtv` | 元数据最全(番号/日期/类型/系列/厂商/演员/男优/监督)、日期正确、`og:video:*` 规整 | URL 带 `dm{n}` 混淆;页面重(320KB)、有反爬混淆 JS |
| **P2** | javxxx.me | `JAVXXX` | `javxxx` | WP 三站中**唯一有显式 `Code:` 番号字段**;`/ja/video/{番号}/` 实测 200 直出;日文演员名;ISO 日期格式 | 日期值错误(收录日);`href` 带 `/v{n}/` 前缀,选择器须用 `contains` |

> **WP 三站(javhdporn / javxxx / bestjavporn)是同一运营方,只接一个,已选定 javxxx。**
> 选型依据见下。挑选标准:**番号字段是否独立可读**(引擎会丢弃 `Number` 为空的搜索结果,靠标题切番号很脆弱)。

**WP 三站选型实测对比**

| 判据 | javhdporn | **javxxx** ✅ | bestjavporn |
| --- | --- | --- | --- |
| 番号直出 URL | `ssis-001` 301 → `/ja/v1/video/…`;`svdvd-907` 200 | **`ssis-001`、`svdvd-907` 均 200** | 两个样本都 301,需跳转 |
| 番号字段 | ❌ 无,只能从标题切 | ✅ **显式 `<span>Code:</span>`** | ✅ `h4.entry-title` |
| 演员名 | ✅ 日文 | ✅ 日文 | ❌ 部分罗马字(`乙白さやか` → `Otoshiro Sayaka`) |
| 日期格式 | ❌ `2月 3, 2026`(非标准,难解析) | ✅ ISO `2026-06-12` | ⚠️ `2022/08/29` |
| 搜索结果 | `span.duration` | `span.duration` + `span.release` | `span.duration` |
| 详情结构 | `#video-date` / `#video-actors`(ID 稳定) | `div.movie-description > div.text-secondary1` | 同 javhdporn |

> javxxx 胜出的决定性一条是 **`Code:` 字段独立可读**,`Number` 不依赖标题解析;
> javhdporn 结构虽然更整齐(用 `id` 而非 `class`),但没有独立的番号字段,且日期格式最难解析。
> bestjavporn 演员名罗马字化,与「使用日语页避免翻译失真」的约定直接冲突,排除。
>
> P2 定位为「番号直查的兜底源」,**发行日期字段建议直接丢弃或降权**(三站给的都是收录日,非真实发行日)。

---

## 一、通用实现骨架

### 1.1 接口与注册

5 站都实现 `MovieProvider` + `MovieSearcher`:

```go
var (
	_ provider.MovieProvider = (*XXX)(nil)
	_ provider.MovieSearcher = (*XXX)(nil)
)

func init() { provider.Register(Name, New) }
```

并在 `engine/register.go` 按字母序追加 blank import(注意 `10musume`/`18av`/`1pondo` 已打头):

```go
_ "github.com/metatube-community/metatube-sdk-go/provider/javtrailers"
_ "github.com/metatube-community/metatube-sdk-go/provider/njavtv"
```

### 1.2 构造器

5 站都要用**日语页**以保元数据准确(理由同 18av:中文/英文页会翻译演员名与类型名)。
`scraper.NewDefaultScraper` 默认已带随机浏览器 UA(`WithRandomUserAgent`)——
**这是绕过 Cloudflare 的唯一必要条件**,不要手动改成固定 UA。

```go
func New() *XXX {
	return &XXX{
		scraper.NewDefaultScraper(Name, baseURL, Priority, language.Japanese,
			scraper.WithHeaders(map[string]string{
				"Referer": baseURL,
			}),
		),
	}
}
```

### 1.3 三个 ID 相关方法的两种范式

| 范式 | 适用 | 实现 |
| --- | --- | --- |
| **A. 直拼** | 番号能直接拼出详情页 URL | `GetMovieInfoByID` 直接 `GetMovieInfoByURL(fmt.Sprintf(detailURL, id))` |
| **B. search-then-fetch** | 番号拼不出 URL(详情页 URL 含内部 ID) | 先 `SearchMovie(id)`,取品番完全匹配的结果,再 `GetMovieInfoByURL(r.Homepage)` |

范式 B 的参考实现(与 `provider/18av/18av.go` 的 `GetMovieInfoByID` 同构):

```go
func (p *XXX) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	results, err := p.SearchMovie(p.NormalizeMovieKeyword(id))
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, provider.ErrInfoNotFound
	}
	// 优先取品番完全匹配,避免同名系列串味
	target := results[0]
	for _, r := range results {
		if strings.EqualFold(r.Number, id) {
			target = r
			break
		}
	}
	return p.GetMovieInfoByURL(target.Homepage)
}
```

### 1.4 必踩的坑

1. **`MovieSearchResult.Number` 不能为空**——引擎的 `IsValid()` 会直接丢弃 `Number` 为空的搜索结果
   (ggjav 曾因此表现为「搜索无结果」)。5 站的搜索结果标题都以番号开头,切第一个空格即可。
2. **`GetMovieInfoByURL` 里 colly 默认跟随 301/302**,njavtv / bestjavporn 依赖这一点,不要关掉。
3. **`parser.ParseIDToNumber` 对本方案不适用**:`1svdvd00907` → `1SVDVD-00907`(前导零+多余前缀)。
   **一律从页面上的「品番 / DVD ID / 番号」字段取值**,不要用 ID 反推。
4. `parser.ParseRuntime` 已支持 `02:27:00`(冒号式)与 `147 分`(日文单位),直接用。
5. 时间格式各站不同,`parser.ParseDate` 走的是通用时间解析,对 `2月 3, 2026` 这类非标准格式不可靠,
   见各站单独说明。

---

## 二、P0 — javtrailers

### 2.1 基本信息

```go
const (
	Name     = "JAVTRAILERS"
	Priority = 500
)

const (
	baseURL   = "https://javtrailers.com/ja/"
	searchURL = "https://javtrailers.com/ja/search?q=%s"
)
```

### 2.2 URL 与 ID

- 详情页:`/ja/video/{DVD_ID}`,如 `/ja/video/1svdvd00907`、`/ja/video/ssis00001`
- `DVD_ID` 是站内 ID,**与番号无稳定推导关系**(`ssis00001` 无 `1` 前缀,`1svdvd00907` 有)→ 用**范式 B**
- **ID 取 URL 最后一段**(即 `DVD_ID`)
- ⚠️ `/ja/video/ssis-001`(番号形式)**返回 404**,不能直拼

```go
func (p *JavTrailers) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	return segs[len(segs)-1], nil // /ja/video/{id}
}
```

### 2.3 搜索

```
GET https://javtrailers.com/ja/search?q=SVDVD-907
```

结果卡片结构(实测):

```html
<div class="col-4 col-md-3 col-lg-2"><div class="card-container">
  <a href="/ja/video/1svdvd00907" class="video-link" title="{英文标题}">
    <div class="card video-card">
      <div class="card-img-container">
        <img data-src="https://images.javtrailers.com/digital/video/1svdvd00907/1svdvd00907ps.w360.webp"
             alt="SVDVD-907 jav" class="card-img-top video-image">
        <span class="badge text-bg-secondary duration-badge">10:23:00</span>
      </div>
      <div class="card-body cb"><div class="card-text">
        <p class="card-text title mb-0 vid-title">SVDVD-907 姫ビッチなGALと…</p>
        <small class="card-text title text-muted">23 Feb 2022</small>
      </div></div>
    </div>
  </a>
</div></div>
```

选择器与字段映射:

| 字段 | 取法 |
| --- | --- |
| 迭代项 | `//div[contains(@class,'card-container')]` |
| href | `.//a[contains(@class,'video-link')]/@href` |
| **Number** | `@alt` 去掉尾部 ` jav`,如 `"SVDVD-907 jav"` → `SVDVD-907`;或 `p.vid-title` 切第一个空格 |
| Title | `//p[contains(@class,'vid-title')]` 文本(日文,含番号前缀,可复用 18av 的 `trimNumberPrefix`) |
| ThumbURL | `img/@data-src` |
| CoverURL | 把 `ps.w360.webp` 替换为 `pl.w800.webp` |
| ReleaseDate | `//small[contains(@class,'text-muted')]`,`23 Feb 2022` |
| Runtime | `span.duration-badge`,`10:23:00` |

> ⚠️ `a/@title` 是**英文标题**,不要用;日文标题在 `p.vid-title`。

### 2.4 详情页

页面有两处结构化数据,互为兜底:

**(1) ld+json `VideoObject`**

```json
{"@type":"VideoObject","name":"SSIS-001 Jav",
 "contentUrl":"https://javtrailers.com/ja/video/ssis00001",
 "duration":"PT02H27M00S",
 "thumbnailUrl":"https://images.javtrailers.com/digital/video/ssis00001/ssis00001pl.w800.webp",
 "uploadDate":"2021-02-17T10:00:00+09:00",
 "description":"SSIS-001 一ヶ月間の禁欲の果てに…"}
```

**(2) 信息块**——统一为 `<p class="mb-1"><span class="font-weight-bold mr-3">标签：</span>值</p>`:

```html
<p class="mb-1"><span class="font-weight-bold mr-3">DVD ID:</span> SSIS-001</p>
<p class="mb-1"><span class="font-weight-bold mr-3">品番：</span> ssis00001</p>
<p class="mb-1"><span class="font-weight-bold mr-3">商品発売日：</span> 17 Feb 2021</p>
<p class="mb-1"><span class="font-weight-bold mr-3">収録時間：</span> 147 分</p>
<p class="mb-1"><span class="font-weight-bold mr-3">監督：</span> 苺原</p>
<p class="mb-1"><span class="font-weight-bold mr-3">メーカー：</span><a href="/ja/studios/s1-no-1-style">…</a></p>
<p class="mb-1"><span class="font-weight-bold mr-3">ジャンル：</span><a href="/ja/categories/…">…</a>…</p>
<p class="mb-1"><span class="font-weight-bold mr-3">出演者：</span><a href="/ja/casts/…">葵つかさ</a>…</p>
```

XPath 模板(可抽成 helper,同 18av 的 `field()`):

```go
field := func(label string) string {
	return fmt.Sprintf(`//p[@class='mb-1'][span[@class='font-weight-bold mr-3' and contains(normalize-space(.), '%s')]]`, label)
}
```

| 字段 | 取法 |
| --- | --- |
| **Number** | `field("DVD ID:")` |
| Title | `//h1[@class='lead']` 文本(完整日文标题) |
| ReleaseDate | `field("商品発売日：")`,`"17 Feb 2021"` → layout `02 Jan 2006` |
| Runtime | `field("収録時間：")` → `parser.ParseRuntime("147 分")` |
| Director | `field("監督：")` |
| Maker | `field("メーカー：")//a` |
| Genres | `field("ジャンル：")//a` |
| Actors | `field("出演者：")//a` |
| CoverURL | ld+json `thumbnailUrl`,兜底 `#thumbnailContainer img/@src` |
| BigCoverURL | 同 CoverURL(`.w800.webp` 即大图) |

**注意 XPath 里「DVD ID:」是半角冒号,其余标签是「：」全角冒号**——`contains()` 用不含冒号的部分更稳,
或两种都写。

### 2.5 测试用例

```go
func TestJavTrailers_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{
		"SSIS-001",   // 有演员、有多类型
		"SVDVD-907",  // 常规
		"SSIS-698",   // 单演员
	})
}
func TestJavTrailers_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		"https://javtrailers.com/ja/video/ssis00001",
		"https://javtrailers.com/ja/video/1svdvd00907",
	})
}
func TestJavTrailers_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{"SSIS-001", "SVDVD-907"})
}
```

---

## 三、P1 — njavtv

### 3.1 基本信息

```go
const (
	Name     = "NJAVTV"
	Priority = 500
)

const (
	baseURL   = "https://njavtv.com/ja/"
	searchURL = "https://njavtv.com/ja/search/%s"
)
```

### 3.2 URL 与 ID

- 规范详情页:`/dm{n}/ja/{slug}`,如 `/dm26/ja/svdvd-907`、`/dm44/ja/ssis-001`
- **`dm{n}` 按影片固定**(同片多次请求值不变),但**无法从番号推导**
- 短链 `/ja/{slug}` → `301` → `/dm{n}/ja/{slug}`,**colly 默认跟随重定向即可**

因此:

```go
// ID 取 slug(番号小写),稳定且可读
func (p *NjavTV) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	return segs[len(segs)-1], nil // …/ja/{slug}
}

// 范式 A:短链会 301 到规范页,colly 自动跟随,无需搜索
func (p *NjavTV) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	return p.GetMovieInfoByURL(fmt.Sprintf("%s%s", baseURL, id))
}
```

> 若实测发现短链 301 不稳定,退回范式 B(search-then-fetch)。

### 3.3 搜索

```
GET https://njavtv.com/ja/search/SVDVD-907
```

结果条目结构(实测):

```html
<div class="my-2 text-sm text-nord4 truncate">
  <a class="text-secondary group-hover:text-primary"
     href="https://njavtv.com/dm26/ja/svdvd-907"
     alt="svdvd-907">SVDVD-907 姫ビッチなGALと…</a>
</div>
```

| 字段 | 取法 |
| --- | --- |
| 迭代项 | `//div[contains(@class,'truncate')]/a[@alt]` |
| href | `@href` |
| **Number** | `@alt` 转大写(`svdvd-907` → `SVDVD-907`);或标题切第一个空格 |
| ID | 由 href 走 `ParseMovieIDFromURL` |
| Title | 链接文本 |
| ThumbURL | 同级/上级容器内的 `img/@src`(需实测确认层级) |

> 搜索页里有 Dean Edwards packer 混淆的 `eval(function(p,a,c,k,e,d)…)`,是**广告脚本**,与数据无关,忽略。

### 3.4 详情页(元数据最全,优先用 `og:` meta)

```html
<meta property="og:title"        content="SSIS-001 一ヶ月間の禁欲の果てに… 葵つかさ 乙白さやか">
<meta property="og:image"        content="https://fourhoi.com/ssis-001/cover-n.jpg">
<meta property="og:video:release_date" content="2021-02-21">
<meta property="og:video:duration"     content="8821">        <!-- 秒 -->
<meta property="og:video:actor"        content="葵つかさ">     <!-- 可重复 -->
<meta property="og:video:actor"        content="乙白さやか">
<meta property="og:video:director"     content="苺原">
```

以及正文信息块(补 `og:` 没有的类型/厂商/系列/番号):

```html
<div class="space-y-2">
  <div class="text-secondary"><span>配信開始日:</span> <time datetime="2021-02-18T11:00:54+08:00">2021-02-18</time></div>
  <div class="text-secondary"><span>品番:</span> <span class="font-medium">SSIS-001</span></div>
  <div class="text-secondary"><span>女優:</span> <a href="…/actresses/…">葵つかさ</a>, …</div>
  <div class="text-secondary"><span>男優:</span> <a href="…/actors/…">平田司</a></div>
  <div class="text-secondary"><span>ジャンル:</span> <a href="…/genres/…">美乳</a>, …</div>
  <div class="text-secondary"><span>メーカー:</span> <a href="…/makers/…">エスワン ナンバーワンスタイル</a></div>
  <div class="text-secondary"><span>レーベル:</span> <a href="…/labels/…">…</a></div>
  <div class="text-secondary"><span>シリーズ:</span> <a href="…/series/…">…</a></div>
</div>
```

字段映射:

| 字段 | 取法 |
| --- | --- |
| Title | `og:title`(兜底 `//h1`) |
| **Number** | 信息块 `//div[contains(@class,'text-secondary')][span[contains(.,'品番')]]/span[last()]` |
| ReleaseDate | ⚠️ 页面有**两个不同的日期**:`og:video:release_date`(SSIS-001 为 `2021-02-21`)与信息块 `time/@datetime`(`2021-02-18T11:00:54+08:00`)。两者差 3 天,**建议取信息块 `time/@datetime`**(带时区、精度更高),`og:` 作兜底 |
| Runtime | `og:video:duration` 秒 → `/60`(SSIS-001 为 `8821` 秒 = 147 分,与 javtrailers 一致)。**信息块里没有「収録時間」行,只能靠 og 补** |
| Actors | `og:video:actor`(**多值**,XPath `//meta[@property='og:video:actor']/@content`) |
| Director | `og:video:director` |
| Genres | `//a[contains(@href,'/genres/')]` |
| Maker | `//a[contains(@href,'/makers/')]` |
| Label | `//a[contains(@href,'/labels/')]` |
| CoverURL | `og:image`(= `https://fourhoi.com/{slug}/cover-n.jpg`,也可由番号直推) |

**注意**:`og:video:actor` 里同时含女优和男优(实测 SSIS-001 三个 actor 里有「平田司」是男优),
若要区分男女,用信息块的 `女優:` / `男優:` 分别取。

### 3.5 测试用例

同 P0 结构,ID 用 slug 形式(`ssis-001`),URL 用 `https://njavtv.com/ja/ssis-001` 与 `/dm44/ja/ssis-001` 各一条。

---

## 四、P2 — javxxx(WP 三站中选定)

### 4.1 基本信息

```go
const (
	Name     = "JAVXXX"
	Priority = 450
)

const (
	baseURL   = "https://www.javxxx.me/ja/"
	searchURL = "https://www.javxxx.me/search/%s/" // 不带语言前缀,实测 200
)
```

### 4.2 URL 与 ID(范式 A)

- 详情页:`/ja/video/{番号小写}/`,如 `/ja/video/ssis-001/`
- **实测 `ssis-001` 与 `svdvd-907` 均直接 200**;个别番号(如 `midv-001`)会 `301` 到
  `/ja/v3/video/midv-001/` 这类带 `/v{n}/` 前缀的路径,colly 跟随即可
- `og:url` 实测为干净路径 `https://www.javxxx.me/ja/video/ssis-001/` → **ID = 番号大写**

```go
func (p *JavXXX) NormalizeMovieID(id string) string {
	return strings.ToUpper(strings.TrimSpace(id))
}
func (p *JavXXX) ParseMovieIDFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 {
		return "", provider.ErrInvalidURL
	}
	return strings.ToUpper(segs[len(segs)-1]), nil // …/video/{番号}
}
func (p *JavXXX) GetMovieInfoByID(id string) (*model.MovieInfo, error) {
	return p.GetMovieInfoByURL(fmt.Sprintf("%svideo/%s/", baseURL, strings.ToLower(id)))
}
```

> ⚠️ 个别番号存在**变体条目**(实测搜 `SSIS-001` 返回 `ssis-001` 与 `ssis-001c` 两条)。
> `ParseMovieIDFromURL` 对变体会得到 `SSIS-001C`,与真实番号不符。
> 若要求 `Number` 精确,**在 `GetMovieInfoByURL` 里以页面 `Code:` 字段覆盖 `info.Number`**
> 而不是依赖 slug(见 4.4)。

### 4.3 搜索

`GET /ja/?s={kw}` → `301` → `/search/{kw}/`(重定向后丢语言前缀)。**直接用 `/search/{kw}/`,实测 200 可用**。

结果卡片(实测):

```html
<div class="col-md-6 col-lg-4 col-xl-3 item thumb-block" data-video-uid="1">
  <div class="video-img">
    <a href="https://www.javxxx.me/video/svdvd-907/" title="{英文标题}">
      <img class="img-fluid image-full video-with-trailer"
           src="https://pics.pornfhd.com/s/digital/video/1svdvd00907/1svdvd00907pl.jpg"
           alt="{英文标题}"
           data-mediabook="https://video.pornfhd.com/p/9/29/svdvd-907/preview.png">
      <span class="release">2025-11-09</span>
      <span class="duration"><i class="fas fa-clock"></i> 10:23:00</span>
    </a>
  </div>
  <h3 class="video-desc mr-3"><a href="…/video/svdvd-907/">SVDVD-907 Constant Fuc…</a></h3>
</div>
```

| 字段 | 取法 |
| --- | --- |
| 迭代项 | `//div[contains(@class,'item') and contains(@class,'thumb-block')]` |
| href | `.//a/@href`(卡片内有两个同 href 的 `a`,取第一个即可) |
| **Number** | 由 href 末段转大写(`svdvd-907` → `SVDVD-907`) |
| ThumbURL | `.//img/@src` |
| CoverURL | ThumbURL 去掉路径中的 `/s/` |
| Runtime | `.//span[@class='duration']` → `parser.ParseRuntime("10:23:00")` |
| ReleaseDate | `.//span[@class='release']`,⚠️ 值是收录日,不准确 |
| Title | 搜索结果里 `a/@title` 与 `h3.video-desc` 文本**都是英文**;日文标题只在详情页 → 搜索结果 Title 作占位,详情页覆盖 |

### 4.4 详情页

```html
<div id="player-tool" class="movie-title mt-3 bg-gray-800 rounded-xl">
  <h1 class="py-2 text-white">SSIS-001 [英語字幕] 一ヶ月間の禁欲の果てに… 葵つかさ 乙白さやか （ブルーレイディスク）</h1>
</div>
<div class="movie-description mt-3 bg-gray-800 rounded-xl">
  <div class="text-secondary1"><span>Code:</span> SSIS-001</div>
  <div class="text-secondary1"><span>配信開始日:</span> 2026-06-12</div>
  <div class="text-secondary1"><span>収録時間:</span> 02:27:00</div>
  <div class="text-secondary1"><span>AV女優:</span>
    <a class="text-nord13" href="…/ja/pornstar/sayaka-otoshiro/" title="乙白さやか">乙白さやか</a>,
    <a class="text-nord13" href="…/ja/v9/pornstar/tsukasa-aoi/" title="葵つかさ">葵つかさ</a>
  </div>
  <div><span class="text-secondary1">メーカー:</span><a href="…/ja/v14/studio/s1-no-1-style/">S1 NO.1 Style</a></div>
  <div class="text-secondary1"><span>レーベル:</span><a href="…/ja/v29/category/censored/">有修正</a>, …</div>
</div>
<meta property="og:url"   content="https://www.javxxx.me/ja/video/ssis-001/">
<meta property="og:image" content="https://pics.pornfhd.com/digital/video/ssis00001/ssis00001pl.jpg">
```

标签行统一为 `<div class="text-secondary1"><span>标签:</span> 值</div>`,可抽 helper:

```go
// 定位「标签:」所在行(值可能是纯文本或 <a>)
row := func(label string) string {
	return fmt.Sprintf(`//div[contains(@class,'text-secondary1') and span[starts-with(normalize-space(.), '%s')]]`, label)
}
```

| 字段 | 取法 |
| --- | --- |
| Title | `//div[@id='player-tool']//h1`;⚠️ 含 `[英語字幕]` 等站点标注,建议 `trimNumberPrefix` 去番号前缀 |
| **Number** | `row("Code:")` 行内 `<span>` 之后的文本,**最可靠**;不要用 slug(变体条目会带 `1c` 之类后缀) |
| ReleaseDate | `row("配信開始日:")`,⚠️ **是收录日,不准确,建议丢弃或降权** |
| Runtime | `row("収録時間:")` → `parser.ParseRuntime("02:27:00")` |
| Actors | `//div[contains(@class,'text-secondary1')]//a[contains(@href,'/pornstar/')]` |
| Maker | `//a[contains(@href,'/studio/')]` |
| Label | `//a[contains(@href,'/category/')]`(有修正、英語字幕 等) |
| Genres | `//a[contains(@href,'/tag/')]`(美少女、美乳、ドラマ 等) |
| CoverURL | `og:image` |

> ⚠️ **演员/厂商/分类的 `href` 都带随机 `/v{n}/` 前缀**(实测同一页面内并存 `/v5/`、`/v6/`、`/v7/`、
> `/v9/`、`/v14/`、`/v25/`、`/v28/`、`/v29/`)。**选择器必须用 `contains(@href,'/pornstar/')`,
> 绝不能用 `starts-with`**,否则会被前缀打断。

### 4.5 测试用例

```go
func TestJavXXX_GetMovieInfoByID(t *testing.T) {
	testkit.Test(t, New, []string{"SSIS-001", "SVDVD-907", "MIDV-001"})
}
func TestJavXXX_GetMovieInfoByURL(t *testing.T) {
	testkit.Test(t, New, []string{
		"https://www.javxxx.me/ja/video/ssis-001/",
		"https://www.javxxx.me/ja/v3/video/midv-001/", // 带 /v{n}/ 前缀的规范路径
	})
}
func TestJavXXX_SearchMovie(t *testing.T) {
	testkit.Test(t, New, []string{"SSIS-001", "SVDVD-907"})
}
```

---

## 五、附:未选中的同源站(javhdporn / bestjavporn)

这两站与 javxxx 同运营方(共用 `pics.pornfhd.com` 图床、同一套 WP 主题),**不单独接入**。
若 javxxx 失效需切换,以下为备用信息。

### 5.1 javhdporn.net(可选备用)

```go
const (
	Name     = "JAVHDPORN"
	Priority = 400 // 备用源,低于 javxxx
)
const (
	baseURL   = "https://www.javhdporn.net/ja/"
	searchURL = "https://www.javhdporn.net/search/%s/"
)
```

- 详情页 `/ja/video/{番号小写}/`:实测 `svdvd-907` 200、`ssis-001` 301 → `/ja/v1/video/ssis-001/`
- **无独立番号字段**,`Number` 只能从标题切第一个空格

```html
<div class="video-description"><div class="desc"><p>SSIS-001 [英語字幕] …</p></div></div>
<div id="video-date">
  <i class="fa fa-calendar"></i> 配信開始日: 2月 3, 2026
  <span class="m-l-15"><i class="fa fa-clock-o"></i> 収録時間: 02:27:00</span>
</div>
<div id="video-actors"><i class="fa fa-user"></i> AV女優: <a href="…/pornstar/…">乙白さやか</a>, …</div>
<div id="video-actors"><i class="fa fa-film"></i> メーカー: <a href="…/studio/…">S1 NO.1 Style</a></div>
<div id="video-actors"><i class="fa fa-tag"></i> レーベル: <a href="…/category/…">有修正</a>, …</div>
```

| 字段 | 取法 |
| --- | --- |
| Title | `//div[@class='video-description']//p` |
| **Number** | 标题切第一个空格(无独立字段) |
| ReleaseDate | `#video-date` 内 `配信開始日:`,⚠️ 收录日;格式 `2月 3, 2026` 需自定义解析 |
| Runtime | `#video-date` 内 `収録時間:` → `parser.ParseRuntime` |
| Actors | `//div[@id='video-actors']//a[contains(@href,'/pornstar/')]` |
| Maker | `//a[contains(@href,'/studio/')]` |
| Genres/Label | `//a[contains(@href,'/category/')]` |
| CoverURL | `og:image` |

搜索卡片:`//article[contains(@class,'thumb-block')]` → `.//a[@class='archive-entry']/@href`;
结果同样只有英文标题,日文标题需进详情页取。

### 5.2 bestjavporn.com(排除,不建议接入)

- 详情页 `/ja/video/{番号小写}-{英文标题slug}/`,如 `/ja/video/ssis-001-decensored/`
- **短链 `/ja/video/{番号}/` 实测全部 301**(`ssis-001`、`svdvd-907` 均需跳转)
- `ParseMovieIDFromURL` 需从「番号-英文slug」切前缀:

```go
// ssis-001-decensored → SSIS-001
var numberRe = regexp.MustCompile(`(?i)^([a-z]+-\d+)`)
```

- 详情页虽好(`//h4[@class='entry-title']` 直接是番号),但:

**排除理由**

1. **演员名部分罗马字化**(实测 `乙白さやか` → `Otoshiro Sayaka`),与「日语页保真」约定直接冲突
2. 番号直出 URL **全部**需要 301
3. 无 ld+json `VideoObject`

---

## 六、公共工具(建议放到 `provider/internal/scraper` 或各包内复制)

### 6.1 封面/缩略图推导(WP 三站通用)

WP 三站图片由 **DMM `content_id`** 决定:

```
番号 SSIS-001  →  content_id ssis00001  (字母小写 + 数字补零到 5 位)
封面  https://pics.pornfhd.com/digital/video/{content_id}/{content_id}pl.jpg
缩略  https://pics.pornfhd.com/s/digital/video/{content_id}/{content_id}pl.jpg
```

但这个推导**只对标准番号成立**(数字位数不固定、`h`/`-uncensored-leaked` 等变体会失败),
**优先取页面上的 `og:image`**,推导仅作兜底。

### 6.2 去番号前缀(复用 18av 的实现)

```go
// provider/18av/18av.go 的 trimNumberPrefix 可直接复制到各包
// 或后续抽到 common/parser 作为 ParseIDToNumber 的配套函数
func trimNumberPrefix(title, number string) string
```

### 6.3 统一「标签行」helper

P0(javtrailers)、P1(njavtv)、P2(javxxx)都有「label: value」结构,建议各包内定义一个
`field(label)` / `row(label)` 生成 XPath,避免字符串散落(参考 `provider/18av/18av.go` 的 `field()`)。

---

## 七、实施顺序与验收

### 7.1 建议顺序

1. **javtrailers(P0)** — 数据结构最规整,先把范式 B + 选择器跑通
2. **njavtv(P1)** — 验证 `og:video:*` 与 301 重定向
3. **javxxx(P2)** — 验证范式 A 与 WP 选择器(`Code:` 字段取番号)

> javhdporn / bestjavporn 不实施(同源,见 §五)。若 javxxx 失效再切 javhdporn。

### 7.2 每站验收清单

- [ ] `go build ./...` + `go vet ./provider/<pkg>/` 通过
- [ ] `Test<X>_Registered` 通过(用 `provider.RangeMovieFactory` 守卫,见 18av 测试)
- [ ] `GetMovieInfoByID` 对 3 个番号返回非空 `Number`/`Title`/`CoverURL`
- [ ] `GetMovieInfoByURL` 对 2 个详情页 URL 通过
- [ ] `SearchMovie` 返回结果且每条 `Number` 非空(否则引擎会丢弃)
- [ ] `ParseMovieIDFromURL` 单测覆盖:正常 URL、带 `/v{n}/` 前缀的 URL、非法 URL
- [ ] 接入 `engine/register.go`
- [ ] **javxxx 的 `Number` 必须取自 `Code:` 字段**,不得用 slug(变体条目会带 `1c` 后缀)
- [ ] **日期字段实测比对真实发行日期**(javxxx 此项必然失败,需决定丢弃还是保留)

### 7.3 已知风险

| 风险 | 说明 | 应对 |
| --- | --- | --- |
| Cloudflare 挑战 | 全部 5 站对非浏览器 UA 返回 403 | `NewDefaultScraper` 自带随机 UA,保持默认即可 |
| 机房 IP 被拦 | 实测走住宅代理,机房 IP 未验证 | 上线后观察 403 率;失败则降优先级 |
| 站点改版 | WP 主题/字段变更 | 选择器尽量用 `contains(@class,…)` 而非全等匹配 |
| 数据污染 | javxxx 日期是收录日、标题带 `[英語字幕]` 标注 | P2 定位为兜底源,优先级低于 P0/P1;日期字段丢弃或降权 |
| 同源重复 | javhdporn / javxxx / bestjavporn 为同一运营方 | 只接 javxxx 一个 |

---

## 八、附:调研半成品文件位置

结构分析用的原始 HTML 快照(本地临时):`%TEMP%\plan\{jt,nj,jh,jx,bj}_{detail,search}.html`。
接入完成后应清理,保持仓库干净。
