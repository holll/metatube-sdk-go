# AVWiki Provider 实施方案

**目标：** 在 `feat/avwiki-provider` 分支新增 AV-Wiki 影片来源，提供番号检索、基础资料及第三方演员识别。

**架构：** 使用现有 Scraper/Colly 获取 HTML，用 goquery 精确解析当前作品正文的 `dl.dltable`；搜索解析本站详情链接及卡片内容。返回独立来源记录，不自动覆盖 FANZA 的演员字段，不修改通用引擎合并逻辑。

**技术栈：** Go、Colly、goquery、httptest、testify。

## 范围与取舍

- 对比方案：直接修改 FANZA 做演员覆盖会耦合外部识别与官方数据；新增通用合并引擎超出此次需求；选择独立 `AVWiki` provider（优先级 350）。
- 新增 `provider/avwiki/avwiki.go`、离线测试及 `engine/register.go` 导入。
- 实现 MovieProvider/MovieSearcher，不实现 ActorProvider，不绕过验证码、不下载媒体、不做全站采集。
- 使用作品详情 slug 为 provider ID（`hhsi-024`），Number 取“メーカー品番”，CID 从“FANZA品番”读取用于精确比对，不用 CID 覆盖 Number。
- 番号、CID 的匹配忽略大小写和分隔符，但不删改数字前导零；不凭标题误匹配另一集。
- 按 `dl.dltable` 的 dt/dd 标签提取标题、Label、演员、番号、发行日；演员仅取“AV女優名”对应 dd，不解析整页演员链接。
- 样本 HHSI-024：标题「あかり」、Number HHSI-024、CID hhsi024、Label えちえち素人、演员识别 皆瀬あかり、发行日 2022-09-25。取图片 alt 或精确后缀清理标题，勿将整篇“是谁”文章标题当影片名。
- Size/年龄是作品化名档案，不写入影片 Runtime 等无关字段；不生成假的简介、时长和评分。
- 封面限定当前作品主图，处理 data-src 懒加载；不能取推荐作品、图标或 data:image 占位。保留图片 URL 参数，不盲猜大图地址。
- 演员占位值 `(≥o≤)`、`＊＊＊`、空值、不明等过滤，不把识别结果声明为官方确认。
- URL 仅允许 av-wiki.net 的 HTTP(S) 单段作品路径；拒绝首页、搜索、归档/演员页面、外部主机、畸形 ID，解析合法 canonical 并保留规范 slug。
- 验证页即使 HTTP 200 也返回明确错误，不等价于空搜索。搜索结果中作品标题可能链接 FANZA 广告，本站详情入口实际为“続きを読む”，必须选本站作品链接。

## 步骤 1：离线测试先行

文件 `provider/avwiki/avwiki_test.go`。

写最小 HTML fixture，测试 HHSI-024 的 dt/dd 映射、主图懒加载、广告链接与相关推荐排除、多演员去重和占位过滤。搜索测试覆盖“続きを読む”本站详情链接、正常空结果、HTTP 200 验证页、相近番号不可误命中、CID 查询能定位到实际 slug。用自定义 RoundTripper/httptest 覆盖公共 HTTP 方法、非 2xx、404、URL 参数编码，不依赖 live 网络。

先运行 `go test ./provider/avwiki -count=1`，确认未实现接口时报错。

## 步骤 2：实现 provider

新增 `provider/avwiki/avwiki.go`；使用 Scraper，保持代理/timeout 设置有效，支持实际采集方法而非测试专用旁路。标准 slug 可直接取详情，非标准输入通过站内搜索精确验证番号/CID再回查。不能根据任意搜索结果取第一条。

404 -> ErrInfoNotFound；不完整详情 -> ErrIncompleteMetadata；HTTP/网络错误和挑战页应返回原始/独立错误，不通过其他接口绕过拦截。

## 步骤 3：注册与验证

在 `engine/register.go` 增加 blank import。在包内验证 provider factory 已注册。

执行：

```sh
go test ./provider/avwiki -count=1
go vet ./provider/avwiki ./engine
go build ./...
git diff --check
```

离线测试必须不触网；少量 smoke 测试仅通过 `127.0.0.1:7890` 代理运行，遇到验证页停止，不增加绕过逻辑。Go 下载依赖同样设置代理。所有更改保留在独立 worktree，不提交、不合并。

## 实施与验收记录（2026-10-21）

已实现上述范围。规范 slug 回查以 canonical ID 或番号/CID 精确一致为依据，修复了 CID 查询得到实际 slug 后无法按返回 ID 再取详情的问题。标题过滤仅针对确定的认人问句，不因合法片名含“誰”而误删。

验收通过：

- `go test ./provider/avwiki -count=1 -cover`，覆盖率 93.4%。
- `go vet ./provider/avwiki ./engine`、`go build ./...`、`git diff --check` 均通过。
- `go test -race ./provider/avwiki -count=1` 通过；覆盖带横杠、无横杠和下划线 canonical slug 的 ID 回查。
- 独立使用本地真实 HTML 回放：HHSI-024 详情、HHSI-024 搜索、CID 搜索验证页，全部符合预期；验证页明确返回 ErrChallenge。
- 使用临时 Go overlay 与 Konodare 一起构建、验证共同注册及 URL 路由，无需合并分支或修改 main。

访问限制：再次抽查无匹配搜索也返回验证页，未继续重试或绕过。因此真实正常空结果模板尚未确认；当前仅严格识别 main 中明确无结果提示，其余未知模板保守返回 ErrIncompleteMetadata，不伪装成正常空结果。测试中的最小空页为人工 fixture，不代表已验证真实空结果页面。网站实时可用性不作为离线验收通过的承诺。

识别出的演员属于第三方来源；本次未实现与 FANZA 的跨源合并。组合路由验证还发现既有 FANZA 的主域名是 `www.dmm.co.jp`，没有将 `video.dmm.co.jp` 注册到引擎主机表：其 provider 内的素人详情 GraphQL 路径可用，不等于引擎可直接按新域名 URL 选中它。本次不修改 FANZA，作为独立后续问题记录。

全仓测试编译仍受基线 `engine/dbengine` 的 Windows dockertest 类型兼容问题阻塞，本次未改依赖。所有文件保留在本分支 worktree，未提交。
