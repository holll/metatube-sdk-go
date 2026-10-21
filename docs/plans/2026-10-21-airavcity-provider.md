# AirAVCity Provider 实施方案

**目标：** 在 `feat/airavcity-provider` 分支新增 airavcity4.cc（瘋AV / airav.io）影片来源，提供番号检索与基础资料。

**架构：** 沿用现有 Scraper/Colly 获取 HTML，用 goquery 解析搜索结果卡片与详情页字段。返回独立来源记录，不实现 ActorProvider，不做跨源合并。

**技术栈：** Go、Colly、goquery、httptest、testify。

## 范围与取舍

- 对比方案：与 jpfhw（字段最少、片库旧）、javhoo（需代理、片库旧）、avgood（含磁力链接）相比，airavcity4 字段最全（番号、标题、简介、演员、标签、厂商、封面、日期），且本机直连可用。
- 新增 `provider/airavcity/airavcity.go`、离线测试及 `engine/register.go` 导入。
- 注册名 `AirAVCity`，优先级 300（低于 FANZA 1001、Konodare 400、AVWiki 350）。
- 实现 `MovieProvider`/`MovieSearcher`，不实现 ActorProvider。

## 站点结构（已实测）

- 搜索：`GET /search_result?kw={keyword}`，结果卡片为 `div.col.oneVideo`，内含：
  - 详情链接 `a[href^="/video?hid="]`
  - 标题 `div.oneVideo-body h5`，形如 `STARS-087 永野一夏 AV出道`
  - 封面 `img.card-img-top[src]`，形如 `/storage/cover/big/{hid}.jpg?{ts}`
  - 分页参数是 `idx`（`?kw=X&idx=2`），每页 60 条
- 详情：`GET /video?hid={hid}`，字段：
  - 番号 `li.my-2` 内 `番號：<span>STARS-087</span>`
  - 演员 `li.my-2 a[href^="/actor?id="]`
  - 标签 `li.my-2 a[href^="/tag?tid="]`
  - 厂商 `li.my-2 a[href^="/tag?fid="]`
  - 日期 `div.video-item` 内 `i.fa-clock` 所在 div 文本 `2019-06-20 00:00:00`
  - 标题 `div.video-title h1`
  - 简介 `div.video-info > p.my-3`
  - 封面 `meta[property="og:image"]`（推荐作品也用 `/storage/cover/big/`，全局取第一张会取错）
- 空结果：`div.row.row-cols-2` 内无 `div.col.oneVideo`
- 不存在 hid：HTTP 404
- 挑战页：经代理访问会返回 403 且标题为 `Just a moment...`（Cloudflare），需独立错误

## 关键设计

**ID 与番号分离。** 站点详情 URL 用 `hid`（`99-21-14639`、`QC-BT-4734904`、`127999`），与番号无对应关系。因此：

- `ID` = hid，小写规范化，形如 `[a-z0-9]+(-[a-z0-9]+)*`
- `Number` = 详情页 `番號` 字段
- `Homepage` = `https://airavcity4.cc/video?hid={hid}`

`GetMovieInfoByID` 按输入形态分流：

- 匹配 `^[A-Za-z][A-Za-z0-9]*-[0-9]+$`（标准番号，如 `FNS-266`、`SSIS-001`）→ 走搜索，取番号完全一致的卡片再取详情；无匹配返回 `ErrInfoNotFound`
- 其他形态（hid，如 `99-21-14639`、`QC-BT-4734904`）→ 直接取详情

**标题解析。** 搜索卡片标题以番号开头，用 `^([A-Za-z0-9]+(?:-[A-Za-z0-9]+)*)\s+` 提取番号；提取不到说明是系列合集等非单部作品，跳过该卡片。实测 `STARS` 关键词 60 条中有 8 条属于此类。

**封面选择。** 详情页必须用 `og:image`，因为推荐作品同样使用 `/storage/cover/big/` 路径，按选择器取第一张会命中推荐位。

**不抓取的内容。** 详情页含 HLS 播放地址（JSON-LD `contentUrl`）与跳转播放站链接，均不写入任何字段。

## 步骤 1：离线测试先行

文件 `provider/airavcity/airavcity_test.go`。

用最小 HTML fixture 覆盖：

- 详情解析：番号、标题、演员多值、标签、厂商、日期、简介、封面取 og:image
- 搜索结果：卡片解析、番号前缀提取、非单部作品跳过、同番号去重
- 空结果：无卡片且页面结构正常 → 返回空列表
- 挑战页：标题含 `Just a moment...` → 独立错误，不当作空结果
- 404 → `ErrInfoNotFound`
- URL 边界：拒绝首页、搜索页、外部主机、伪造后缀主机、畸形 hid
- 详情字段缺失 → `ErrIncompleteMetadata`

用自定义 `RoundTripper` 覆盖公共方法，不依赖外网。

## 步骤 2：实现 provider

新增 `provider/airavcity/airavcity.go`，使用 `scraper.NewDefaultScraper`，保留代理与超时配置。重定向处理器只允许 `airavcity4.cc` 的详情与搜索路径。

错误映射：

- 404 → `ErrInfoNotFound`
- 挑战页 → `ErrChallenge`（包级导出，便于上层识别）
- 字段不全导致 `IsValid()` 失败 → `ErrIncompleteMetadata`
- 网络错误原样返回

## 步骤 3：注册与验证

`engine/register.go` 增加 blank import。执行：

```sh
go test ./provider/airavcity -count=1 -cover
go vet ./provider/airavcity ./engine
go build ./...
git diff --check
```

离线测试不触网。少量 smoke 仅本机直连（该站走代理会被 Cloudflare 拦），不增加绕过逻辑。更改保留在独立分支，不提交、不合并。
