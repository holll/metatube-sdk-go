# Konodare Provider 实施方案

**目标：** 在 `feat/konodare-provider` 分支新增一个兼容 `konodare.wew.jp` 和 `konodare.jp` 的影片 provider。

**架构：** 使用项目现有 scraper/Colly 和 goquery，两个 HTML 模板独立解析，共用 provider 身份。旧站搜索优先，仅在有效空结果时查询新站；网络错误、验证码和模板异常不得当作空结果。按真实详情 URL 获取数据，不猜测 Getchu 编号与番号关系。不做跨 provider 合并。

**技术栈：** Go、Colly、goquery、httptest、testify。

## 范围与取舍

- 对比方案：只接旧站维护成本低但缺少新站覆盖；两个 provider 会重复搜索和缓存；选择单 provider 双站适配。
- 新增 `provider/konodare/konodare.go` 与离线测试，注册名 `Konodare`，优先级 400（低于官方源）。
- 实现 `MovieProvider`、`MovieSearcher`；不实现 ActorProvider，不抓取视频文件，不调用全量角色 JSON。
- ID 使用稳定的域名标识加 slug（例如 `wew:pnme-333`、`jp:cosh-308`），确保回查站点明确；同次搜索按规范番号去重。URL 别名/重定向的规范地址须正确回写。
- Number 保留站点番号的语义（INSTC665 不强制变成 INSTC-665）；对输入只统一大小写和空白，不臆造编号。
- 旧站：h1/商品表格为番号，正文 h9 为标题；商品表格按标签解析发行日、时长、厂商和演员；正文简介排除侧栏和相关推荐。
- 新站：使用 `.mdb-work-title`、`.mdb-work-item-no`、`.mdb-summary-row`、`.mdb-work-content`、`.mdb-source-card`、`.mdb-cosplay-meta-row` 等字段。平台信息存在多条时选择第一条有效信息，不把各平台时长相加。
- 演员 `っぽい` 保留，禁止转换成已确认身份。Genre 可包含角色/原作，去重；不混入相关推荐。
- 检查真实图片地址，过滤空值和 data: 占位图，保留来源中的完整 URL，不盲目拼出大图。
- URL 边界：只允许两个明确主机的 HTTP(S) 作品路径，拒绝首页、搜索、分类/演员页、伪造后缀主机、异常 ID。

## 步骤 1：先写离线测试

文件：`provider/konodare/konodare_test.go`。

使用人工裁剪的最小 HTML 样本，覆盖旧站 PNME-333、INSTC665、新站 COSH-308、商品表格 rowspan、重复封面、演员推测后缀、平台日期/时长、空结果和验证页。利用自定义 HTTP RoundTripper 或测试服务器测试实际公共方法，不能仅测未被使用的解析 helper。测试新站 ID 回查不访问旧站、旧站有效空结果才回退、错误不会回退、URL 查询参数编码。

运行 `go test ./provider/konodare -count=1`，预期新增接口尚不存在导致失败。

## 步骤 2：实现最小 provider

新增 `provider/konodare/konodare.go`；遵循现有 Scraper 的代理和 timeout 行为。解析结果必须满足 `MovieInfo.IsValid()`；404 映射 ErrInfoNotFound，验证码/拦截页给独立错误，缺字段给 ErrIncompleteMetadata，不吞错误。

按严格 URL 规则和域名 ID 实现 Parse/Normalize/Get/Search。优先解析响应中的合法 canonical（仅允许本站两个主机的作品路径），不得接受外部 canonical。搜索保持站点结果顺序并校验字段。

## 步骤 3：注册与双域名路由

- `engine/register.go` 增加包导入。
- 现有引擎只将 `Provider.URL()` 的主机注册到路由表。为完成明确需求，在 `provider/provider.go` 增加可选 `URLAliases() []*url.URL` 接口，并在 `engine/init.go` 的影片 host 注册处加入该接口声明的其他主机，避免重复注册。
- 不改变必需 Provider 接口，不改变字段合并、排序或其他 provider 的行为。
- 在 `engine/` 添加离线测试，使用 `engine.New(nil)` 或同包最小初始化验证两域名指向同一 provider、伪造域名不匹配。

## 步骤 4：验证

在本 worktree 执行：

```sh
go test ./provider/konodare -count=1
go test ./engine -run Konodare -count=1
go vet ./provider/konodare ./engine
go build ./...
```

网络请求仅在必要的少量 smoke 验证时使用本机代理；默认测试不依赖外网。Go 依赖下载同样设置 HTTP_PROXY/HTTPS_PROXY。完成后检查 `git diff --check`，不提交、不合并分支。

## 实施与验收记录（2026-10-21）

已实现上述范围，新增可选接口名为 `provider.ProviderURLAliases`。新站同时解析当前作品的 `video source` 为 PreviewVideoURL，仅返回地址，不下载视频；简介保留 br/段落换行。无前缀番号匹配忽略横杠、大小写和空白，但不删除数字前导零；返回 Number 保留站点格式。

真实空结果模板与有结果模板不同，不能使用人工空列表代替：

- 旧站：`body.search-no-results main#main div.posts-not-found > p`，提示“投稿が見つかりませんでした。”，无 `#list`。
- 新站：`main.mdb-main > div.mdb-empty`，提示“に一致する作品が見つかりませんでした。”，无 `.mdb-search-grid`。
- 未知模板、验证码、HTTP/网络错误均不触发新站回退。

验收通过：

- `go test ./provider/konodare ./engine -count=1 -cover`，provider 覆盖率 90.2%；engine 运行双域名注册、别名去重测试。
- `go vet ./provider/konodare ./engine`、`go build ./...`、`git diff --check` 均通过。
- `go test -race ./provider/konodare ./engine -count=1` 通过。
- 独立使用本地保存的真实页面回放 7 个场景：PNME-333、INSTC665、历史 item4063410 canonical、新站 COSH-308、旧站搜索、真实旧站空页到新站命中、两站真实空页。全部通过，回放无外网请求。
- 使用临时 Go overlay 与 AVWiki 一起构建、测试引擎共同注册及 ProviderID 编解码，未合并分支、未修改 main。

全仓测试编译存在基线故障：`engine/dbengine` 的旧版 `dockertest` 在 Windows 下出现 SECURITY_DESCRIPTOR 类型不兼容。本次不调整依赖；不宣称全仓测试通过。文件保留在本分支 worktree，未提交。
