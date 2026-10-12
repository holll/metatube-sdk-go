# JAV 聚合站 provider 候选调研

调研日期:2026-10-10
调研对象:5 个 JAV 聚合/索引站,评估其作为 metatube **数据提供源(provider)** 的可行性。
测试样本:SVDVD-907(有码)、SSIS-001(有名演员)。

## 一、Cloudflare 结论

**5 个站点全部部署 Cloudflare CDN,但都没有对「浏览器 UA 客户端」构成阻断性挑战墙。**

关键实测数据:

| 请求方式 | 结果 |
| --- | --- |
| 代理 + Chrome 131 UA(curl) | 5/5 全部 `200`,返回真实页面 |
| 代理 + Go `net/http` + Chrome UA | 5/5 全部 `200`,返回真实页面 |
| 代理 + curl 默认 UA | 5/5 全部 `403`,`<title>Just a moment...</title>` |
| 代理 + 无 UA 头 | 5/5 全部 `403`,同上 |
| 连续 8 次同 URL(浏览器 UA) | 5/5 各 8/8 通过,未触发速率限制 |

结论:这是**基于 UA 的 bot 过滤**(非浏览器 UA 触发 Managed Challenge),不是 TLS 指纹级别的强校验——
因为 Go 客户端的 TLS 指纹与 Chrome 明显不同,却能正常通过。

metatube 的 `scraper.NewDefaultScraper` 默认启用 `WithRandomUserAgent()`(真实浏览器 UA),
因此**天然满足放行条件**,无需额外反爬处理。

> ⚠️ 遗留风险:实测走的是本机住宅代理出口。metatube 实际多部署在 VPS/机房 IP 上,
> Cloudflare 对机房 IP 的 bot score 更严格,届时是否仍直接放行**未经实测验证**。
> 若未来出现 403,优先考虑降优先级或加备用源,而非投入 JS 挑战破解。

### 各站响应头特征

| 站点 | Server | 其他 |
| --- | --- | --- |
| javhdporn.net | cloudflare | `Cf-Apo-Via: tcache`、`cf-edge-cache: cache,platform=wordpress`、`CF-Cache-Status: HIT` → **WordPress + Cloudflare APO** |
| javxxx.me | cloudflare | 同上(同架构) |
| bestjavporn.com | cloudflare | 同上(同架构) |
| njavtv.com | cloudflare | `x-powered-by: PHP/8.5.10`、`cf-cache-status: HIT` |
| javtrailers.com | cloudflare | `x-powered-by: Nuxt`、`cf-cache-status: DYNAMIC` |

WordPress + APO 三站的 `CF-Cache-Status: HIT` 说明 HTML 由边缘缓存直接吐给匿名访客,
这也是浏览器 UA 能拿到完整页面的原因之一。

## 二、搜索结果

全部 5 站都提供可用的搜索入口。

| 站点 | 搜索请求 | 备注 |
| --- | --- | --- |
| javhdporn.net | `GET /ja/?s={kw}` → `301` → `GET /search/{kw}/` | 重定向后**不带语言前缀** |
| javxxx.me | `GET /ja/?s={kw}` → `301` → `GET /search/{kw}/` | 同上 |
| bestjavporn.com | `GET /zh/?s={kw}` → `301` → `GET /search/{kw}/` | 同上 |
| njavtv.com | `GET /ja/search/{kw}` | 直接 200,无需跳转 |
| javtrailers.com | `GET /ja/search/{kw}` | ✅ **服务端渲染**,结果直接可抓 |

三个 WordPress 站的 `?s=` 会 301 到 `/search/{kw}/`(SEO 路径),colly 默认跟随重定向即可。

> **javtrailers 搜索的坑**:`/ja/search?q={kw}` 返回的是一个客户端渲染的空壳
> (`Search Engine Beta`,结果由 JS 异步加载,HTML 里没有条目)。
> 正确形式是**路径参数** `GET /ja/search/{kw}`(Nuxt 路由定义为 `/search/:searchTerm()`),
> 该形式是 SSR 的,HTML 里直接带结果条目:

```html
<div class="col-4 col-md-3 col-lg-2">
  <div class="card-container">
    <a href="/ja/video/1svdvd00907" class="video-link" title="...">
      <div class="card-img-container">
        <!-- 注意缩略图是懒加载的 data-src,不是 src -->
        <img data-src="https://images.javtrailers.com/digital/video/1svdvd00907/1svdvd00907ps.w360.webp"
             alt="SVDVD-907 jav" class="card-img-top video-image">
        <span class="badge text-bg-secondary duration-badge">10:23:00</span>
      </div>
      <div class="card-body cb"><div class="card-text">
        <p class="card-text title mb-0 vid-title">SVDVD-907 姫ビッチなGALと...</p>
        <small class="card-text title text-muted">23 Feb 2022</small>  <!-- 原始发行日 -->
      </div></div>
    </a>
  </div>
</div>
```

`/ja/search/*` 下**不带语言前缀**的 `/search/{kw}` 也可用。

## 三、详情页 URL 规律与 ID 往返

provider 的关键约束是 `GetMovieInfoByID(id)` 与 `ParseMovieIDFromURL(url)` 必须能互相推导。

| 站点 | 详情页 URL 形态 | 番号可直出 URL? |
| --- | --- | --- |
| javhdporn.net | `/{lang}/video/{番号小写}/` | ✅ `GET /ja/video/svdvd-907/` 直接 200,往返最干净 |
| javxxx.me | `/{lang}/video/{番号小写}/` | ✅ 同上 |
| bestjavporn.com | `/{lang}/video/{番号小写}-{英文标题slug}/` | ⚠️ 短 URL `301` 跳到完整 slug,需跟随重定向;`/ja/` 版路径与 `/zh/` 版完全一致,仅换语言前缀 |
| njavtv.com | `/dm{n}/{lang}/{番号小写}` | ⚠️ 短链 `/{lang}/{番号}` → `301` → `/dm{n}/...`;`dm{n}` 按影片固定(非随机),但必须跟随重定向 |
| javtrailers.com | `/ja/video/{cid}`(如 `1svdvd00907`、`ssis00001`) | ❌ `番号` 形式的 URL 直接 404,只能用 DMM 的 **cid**,须 search-then-fetch |

> javtrailers 的 `cid` 是 **DMM 内容 ID**,前缀不固定、**无法由番号推导**:
> `SVDVD-907 → 1svdvd00907`(带前导 `1`),但 `SSIS-001 → ssis00001`(无前导)。
> 因此 `GetMovieInfoByID` 必须走「搜索 → 取 href」,`ParseMovieIDFromURL` 直接取 URL 末段。

## 四、元数据完整度

| 站点 | 结构化数据 | HTML 字段 |
| --- | --- | --- |
| **javtrailers.com** | ld+json `VideoObject`:`name`/`thumbnailUrl`/`uploadDate`/`duration`/`description` | DVD ID、品番(cid)、商品発売日、収録時間、**監督**、メーカー、ジャンル、出演者 |
| **njavtv.com** | 无 ld+json;`<meta name="keywords">` 混合演员/类型/厂商 | 配信開始日(`<time datetime>` ISO)、品番、女優、**男優**、ジャンル、シリーズ、メーカー、レーベル;有 preview 图 |
| **javhdporn.net** | ld+json `@graph` → `VideoObject`:`name`/`thumbnailUrl`/`duration`/`uploadDate`/`datePublished`/`keywords` | 配信開始日、収録時間、AV女優、メーカー、レーベル(后接类型 tag) |
| **javxxx.me** | ld+json `VideoObject`:`name`/`thumbnailUrl`/`description`/`uploadDate`/`duration`/`keywords`、`actor: []` | Code、配信開始日、収録時間、AV女優、メーカー、レーベル(后接类型 tag) |
| **bestjavporn.com** | 无 `VideoObject` | 商品発売日、収録時間、AV女優、メーカー、类型(纯日文) |

> **修正**:此前初评把 javxxx 记为「元数据较薄」是错的。实测 javxxx 与 javhdporn 的字段一致,
> 同样有 `AV女優`(演员)、`Code`(番号)、`収録時間`、`メーカー`、类型,只是比 javtrailers 少 `監督`。

bestjavporn `/ja/` 与 `/zh/` 字段对照(同一影片 SVDVD-907,实测):

| 字段 | `/zh/` 版 | `/ja/` 版 |
| --- | --- | --- |
| 发行日 | 發布日期: 2022/06/11 | 商品発売日: 2022/06/11 |
| 时长 | 時長: 623 分鐘 | 収録時間: 623 分 |
| 厂商 | 片商: Sadistic Village | メーカー: Sadistic Village |
| 类型 | 中出 / 高畫質 / 獵豔 | 中出し / ハイビジョン / ナンパ |
| 播放量 | 1203 次觀看 | 1211 再生数 |

```text
# 日语版详情页(与 /zh/ 版同 slug,只换语言前缀)
https://www.bestjavporn.com/ja/video/svdvd-907-constant-fuck-with-princess-bitch-gal-forget-details-and-fuck-her-heart/
# 搜索:?s= 会 301 到 /search/{kw}/(重定向后不带语言前缀)
https://www.bestjavporn.com/ja/?s=SVDVD-907  →  https://www.bestjavporn.com/search/SVDVD-907/
```

> 按既有约定(见 18av provider),**一律使用 `/ja/` 日语页**,避免中文页把演员名/类型名翻译后失真
> (如「女子校生」被译作「高中女生」)。5 站均支持 `/ja/` 前缀,javhdporn / javxxx / bestjavporn 的
> `/ja/` 详情页与搜索均已实测返回 200。

## 五、数据准确性实测(关键)

用两部影片交叉验证,**WordPress 三站的「发行日期」不可信**——它们填的是「收录到本站的日期」,
而非影片原始发行日,且三站互相矛盾:

| 影片 | njavtv | javtrailers | javhdporn | javxxx | bestjavporn |
| --- | --- | --- | --- | --- | --- |
| SVDVD-907 | `2022-02-23` | `23 Feb 2022` | `5月 19, 2026` | `2025-11-09` | `2022/06/11` |
| SSIS-001 | `2021-02-18` | `17 Feb 2021` | `2月 3, 2026` | `2026-06-12` | `2022/08/29` |

- njavtv 与 javtrailers **两源互相吻合**(SVDVD-907 完全一致;SSIS-001 差 1 天,属时区),可信。
- javhdporn / javxxx / bestjavporn 三站给出**三个互不相同的值**,且与真实发行日相差数月至数年。
- javxxx 的 `配信開始日` 实际等于其 ld+json 的 `dateModified`(最后修改日),
  `uploadDate` 则是 `datePublished`(收录日)——**页面上根本没有原始发行日**。

字段实测对照(SSIS-001):

| 字段 | javtrailers | njavtv | javhdporn | javxxx | bestjavporn |
| --- | --- | --- | --- | --- | --- |
| 番号 | SSIS-001 | SSIS-001 | SSIS-001 | SSIS-001 | SSIS-001 |
| 发行日 | 17 Feb 2021 ✅ | 2021-02-18 ✅ | 2月3,2026 ❌ | 2026-06-12 ❌ | 2022/08/29 ❌ |
| 时长 | 147 分 ✅ | ✅ `og:video:duration` 8821 秒 | 02:27:00 ✅ | 02:27:00 ✅ | 147 分 ✅ |
| 監督 | ✅ 苺原 | ❌ | ❌ | ❌ | ❌ |
| 厂商 | ✅ | ✅ | ✅ | ✅ | ✅ |
| 演员 | ✅ 葵つかさ / 乙白さやか | ✅ 同(另有男優「平田司」) | ✅ 乙白さやか / 葵つかさ | ✅ 乙白さやか / 葵つかさ | ⚠️ 葵つかさ / **Otoshiro Sayaka**(罗马字) |

> bestjavporn 会把部分演员名**罗马字化**(`乙白さやか → Otoshiro Sayaka`),
> 与「使用日语页避免翻译失真」的约定冲突,是明确的减分项。

## 六、推荐排序

| 排名 | 站点 | 理由 | 主要障碍 |
| --- | --- | --- | --- |
| 1 | **javtrailers.com** | 数据最准最全:唯一提供 `監督`、发行日与 njavtv 互证正确、有 `収録時間`、ld+json `VideoObject`、搜索已确认 SSR 可抓 | 详情页只能用 DMM `cid`,番号无法推导 → 必须 search-then-fetch;搜索结果缩略图是懒加载 `data-src` |
| 2 | **njavtv.com** | 发行日准确、字段多(独有 `男優`、`シリーズ`、`レーベル`)、搜索路径最简 `/{lang}/search/{kw}`、`og:video:duration` 可补时长(实测 8821 秒 = 147 分) | 详情页带 `dm{n}` 混淆前缀需跟随重定向;页面含反爬混淆 JS |
| 3 | **javxxx.me** | WP 三站中**唯一有显式 `Code:` 番号字段**(`Number` 不依赖标题解析);`/ja/video/{番号}/` 实测 200 直出;日文演员名;ISO 日期格式;搜索结果含 `span.duration` + `span.release` | ❌ 发行日期是收录日,与真实值差数月至数年;`href` 带随机 `/v{n}/` 前缀,选择器须用 `contains`;有变体条目(如 `ssis-001c`) |
| 4 | **javhdporn.net** | 结构与 javxxx 同源,选择器用 `id` 更整齐 | ❌ 无独立番号字段(靠标题切);日期格式 `2月 3, 2026` 最难解析;番号直出 URL 部分需 301 |
| 5 | **bestjavporn.com** | 可用,有 `/ja/` 版,`h4.entry-title` 直接给番号 | ❌ 演员名部分罗马字化(失真);番号直出 URL 全部需 301;无 ld+json |

**取舍**:若「数据准确性」优先(与既有 18av provider 的日文页约定一致),做 **javtrailers**;
若要「番号直查最快落地」,做 **javxxx**,但须接受发行日期不准(建议直接丢弃该字段)。

> WP 三站(javhdporn / javxxx / bestjavporn)为同一运营方,**只做 javxxx 一个**。
> 三站选型依据见 [`jav-provider-implementation-plan.md`](./jav-provider-implementation-plan.md) §零。

> 📄 **具体实现方案**(包名 / URL 常量 / 选择器 / 方法伪代码 / 测试用例)见
> [`jav-provider-implementation-plan.md`](./jav-provider-implementation-plan.md)。

## 七、注意事项

1. **全部是聚合/索引站**,内容为第三方嵌入,元数据准确度不如 javbus / jav321 这类信息库型站点。
2. 仓库已有同类 provider:`javbus`、`jav321`、`javfree`、`ggjav`,新增前建议先确认差异化价值。
3. **`Number` 字段必须非空**——metatube 的 `MovieSearchResult.IsValid()` 会丢弃 `Number` 为空的搜索结果
   (ggjav 曾因未填 `Number` 表现出「搜索无结果」)。上述站点搜索结果标题均以番号开头,可从中切分。
4. **务必带浏览器 UA**,否则一律 403。
5. 调研期间观察到:经代理访问这批 Cloudflare 站点时,**TLS 握手会间歇性失败**(`schannel: failed to
   receive handshake` / Go 侧 `EOF`),而同一时刻 `example.com` 正常。判定为代理线路问题而非站点封禁
   ——重试即可恢复。实际 provider 部署时若出现类似抖动,应加请求重试而非当作被封处理。
