# 五个候选 AV 数据站点调研（2026-10-21）

调研目标：判断能否接入为 metatube-sdk-go 的影片 provider。所有测试均在本机完成，未做绕过验证码/反爬的处理。

## 结论速览

| 站点 | 可访问性 | 搜索 | 详情字段 | 结论 |
|---|---|---|---|---|
| www.jpfhw.com | 直连 200 | POST 表单，需 cookie | 番号/标题/日期/时长/演员/类别/封面 | **可接入**，字段最少但干净 |
| airavcity4.cc | 直连 200（代理被 CF 拦） | GET `/search_result?kw=` | 番号/标题/日期/演员/标签/厂商/封面/简介 | **可接入**，字段最全 |
| www.javhoo.com/ja | 需代理（本机直连握手失败） | GET `/ja/search/{kw}` | 番号/标题/日期/时长/导演/厂商/系列/类别/演员/封面/样图 | **可接入**，字段丰富 |
| avgood.com/j/ | 需代理 | GET `/j/s/?q=` | 番号/标题/日期/时长/导演/厂商/类别/演员/封面 | **可接入**，但含磁力链接需谨慎 |
| iqqtv.net | Cloudflare 挑战 | — | — | **不可接入**（`cf-mitigated: challenge`，403） |

## 1. jpfhw.com（极品番号库）

- 主机：`www.jpfhw.com`（移动站 `m.jpfhw.com`）
- 详情 URL：`/fanhao/{NUMBER}.html`，例如 `/fanhao/STARS-087.html`
- 搜索：`POST /e/search/index.php`，字段 `keyboard`、`classid=3`、`show=title,ftitle`、`tempid=1`
  - 返回 302 到 `/e/search/result/?searchid=NNN`，**必须保持 cookie** 才能拿到结果
  - 结果条目：`<li><a href="/fanhao/XXX.html"><div class="M_CB"><img ...></div><div class="M_CC">XXX</div></a></li>`
  - 空结果：页面仅含“男人团信息提示 / 没有搜索到相关的内容”
- 详情字段（`div.M_GBC ul li > strong`）：
  - `作品名：`、`发行日期：`（`2019/06/20`）、`播放时长：`（`135分钟`）、`主演女优：`、`作品番号：`、`发行地区：`、`地区分类：`、`类别：`（逗号分隔）
  - 封面：`div.M_GA img[src]`，为站内 `d/file/...jpg`
- 限制：
  - 搜索仅精确匹配完整番号（`STARS-087` 命中，`stars087` 不命中），适合 ID 回查，不适合模糊搜索
  - 数据量偏旧（首页热门多为 2018-2019 作品），HHSI-024 / FNS-266 均无记录
  - 无简介、无演员链接、无样图

## 2. airavcity4.cc（瘋AV / airav.io）

- 主机：`airavcity4.cc`，**直连可用，走本机代理会被 Cloudflare 拦 403**
- 搜索：`GET /search_result?kw={keyword}`
  - 结果链接：`/video?hid={HID}`，HID 形如 `QC-BT-4734904`、`99-21-14639`
  - 空结果：页面有“的搜尋結果”但结果区为 `0`
- 详情字段：
  - `li.my-2 番號：<span>STARS-087</span>`
  - `li.my-2 女優：<a href="/actor?id=...">永野一夏</a>`
  - `li.my-2 標籤：<a href="/tag?tid=...">720p</a>...`
  - `li.my-2 廠商：<a href="/tag?fid=...">SOD</a>`
  - 标题/简介：`og:title`、`og:description`（繁体中文简介）
  - 封面：`og:image`，如 `https://airavcity4.cc/storage/cover/big/{HID}.jpg`
  - JSON-LD `VideoObject` 提供 `uploadDate`、`duration`、`contentUrl`（HLS）
- 限制：
  - 简介为繁体中文，非日文原名
  - `contentUrl` 是可直接播放的 HLS 地址，**不应写入 provider**（涉及内容分发）
  - HID 与番号无对应关系，必须走搜索

## 3. javhoo.com（JAVHOO）

- 主机：`www.javhoo.com`，多语言 `/ja` `/zh` `/en` `/kr`
- 本机直连 TLS 握手失败，**必须走代理**
- 搜索：`GET /ja/search/{keyword}`（`?s=` 等价）
  - 结果卡片：`div.excerpt`，链接 `https://www.javhoo.com/ja/{number-lowercase}`
  - 空结果：页面含“沒有找到你要的内容！”
- 详情 URL：`/ja/{number}`，如 `/ja/xvsr-123`
- 详情字段（`div.pods_widget_field > h3` + 值）：
  - `識別碼:`、`發行日期:`、`長度:`（分钟）、`導演:`、`製作商:`、`系列:`、`類別:`（多个 `<span><a>`）、`演員:`
  - 标题：`<title>` 与 `h1`，形如 `XVSR-123 官能小説 ... 波木はるか`
  - 封面：`img[src*="pics.javhoo.net"][src*="cover"]`，形如 `https://pics.javhoo.net/2016/cover/5drt_b.jpg`
  - 样图：`https://pics.javhoo.net/{year}/sample/{id}_{n}.jpg`（单页约 10 张）
  - 页面还包含 **磁力链接**（约 18 条），不应抓取
- 限制：字段是繁体中文标签；需代理；FNS-266 无记录（片库偏旧）

## 4. avgood.com（AvGood）

- 主机：`avgood.com`，多语言 `/j`（日）`/e`（英）`/k` `/t` `/c`
- 需代理
- 搜索：`GET /j/s/?q={keyword}`
  - 结果卡片：`a[href^="/c/"][href$=".html"]`，如 `/c/232809.html`
  - 空结果：`相關結果が見つかりません`
- 详情字段：
  - `识别码:`、`发行日期:`、`长度:`（`120分钟`）、`导演:`、`制作商:`、`类别:`（换行分隔）、`演员:`
  - 另有 `商品発売日`、`配信開始日`、`収録時間`、`出演者`、`監督`、`シリーズ`、`メーカー`、`レーベル`、`ジャンル`、`品番`（日文页字段更全）
  - 封面：`img.lazy[data-original]`，如 `/remote/pic/2026/10/17/fns-266_main.jpg`
  - 样图：`/remote/thumb/200x160/pic/down2/sample/...`
- 限制：
  - 页面包含**磁力链接与 BT 种子下载**，provider 只应取元数据，绝不抓取下载链接
  - 同一番号常有多个 `/c/{id}.html`（不同清晰度/字幕），需按番号去重
  - 需代理

## 5. iqqtv.net — 不可接入

- 所有路径（首页、`/zh/`、`/vodtype/1`、`/index.php/ajax/data`）均返回 `403`，响应头 `cf-mitigated: challenge`，页面为 `Just a moment...`
- Cloudflare 交互式挑战，未做任何绕过尝试
- 结论：当前形态无法作为 provider；若后续站点调整防护可再评估

## 接入建议

优先顺序：

1. **airavcity4.cc** — 字段最全（含简介、标签、厂商、封面、演员），直连可用，搜索与详情结构清晰。
2. **javhoo.com** — 字段丰富（含导演、系列、样图），搜索/详情路径规整，但需代理且片库偏旧。
3. **avgood.com** — 字段较全，但必须严格排除磁力/种子信息。
4. **jpfhw.com** — 字段最少、片库最旧，仅适合作为精确番号回查的补充源。
5. **iqqtv.net** — 暂不接入。

通用注意点：

- 四个可接入站点均**不实现 ActorProvider**，演员只作为 `Actors []string` 写入。
- 一律不抓取磁力链接、种子、HLS/视频直链。
- 搜索必须校验返回条目的番号与查询一致，不能取第一条。
- 封面/样图需处理懒加载（`data-src` / `data-original`）与 `data:image` 占位。
- airavcity4 直连可用但代理被拦；javhoo/avgood 反之。provider 不应内置代理，交由引擎配置。
- 站点内容均为第三方转载，数据准确性低于 FANZA 官方源，优先级应低于官方源。
