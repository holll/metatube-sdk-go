# Getchu 完善与 Gyutto 接入 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 完善已有 Getchu 详情读取，再新增独立 Gyutto 详情及标题搜索，不引入跨站自动映射。

**Architecture:** 两个 provider 独立保留各自数字 ID，Number 分别为 GETCHU-<ID> 和 GYUTTO-<ID>。使用共享 Scraper 的传输、代理及超时，通过 goquery 在当前作品容器内解析 HTML；不解决验证、不下载作品、不猜测 PNME 番号。保留现有 Konodare 未提交变更与用户 tmpcheck/。

**Tech Stack:** Go、Colly、goquery、golang.org/x/text（日文 EUC-JP）、httptest 风格 mock RoundTripper。

---

## 范围和已验证证据

- Getchu 已存在于 provider/getchu/getchu.go；无 MovieSearcher。详情样本 4058126 包含标题、OG 图、社团、发行日、样本链接和“動画：20分 写真集：60枚”。搜索尚未打通，首版不新增搜索。
- Gyutto 样本 271421 的 #RightSide 内有 .parts_Mds01 h1、#noMovie .unit_DojinMainPh .ItemPh、.unit_DetailBasicInfo dl.BasicInfo、.unit_SamplePhSmall、.unit_DetailSummary。
- Gyutto 标题搜索入口 /search/search_list.php，category_id=10、set_category_flag=1、mode=search、search_keyword（日文 EUC-JP 编码）。卡片位于 .unit_ItemList .unit_SeachSmall .ListBox .ItemBox，作品链接在 .DefiPhotoName a，完整标题可取该链接下图片 alt。空结果提示为“アダルト作品では、「<关键词>」に一致する作品はありませんでした。”。
- Gyutto 推广路径 /af-13475 与 select_uaflag=1 可以归一为 /i/item271421；允许已知参数，不接受任意外域、端口、userinfo、转义路径或未知参数。
- 两站样本无明确演员字段，Actors 保持空；不把作者/社团映射为演员。

### Task 1: Getchu 离线测试

**Files:** Modify provider/getchu/getchu_test.go。

1. 将必须联网的样本测试改为最小人工 HTML + mock transport。
2. 测试 ID 归一、合法 URL 和无效输入在请求前拒绝。
3. 测试详情字段、时长不误读照片张数、主封面不被侧栏覆盖、样本去重及换行保留。
4. 测试 404、网络失败、HTTP 错误、验证码/年龄确认页、缺标题/封面、外域重定向。
5. Run: `go test ./provider/getchu -count=1`，确认新增断言先失败。

### Task 2: Getchu 最小修复

**Files:** Modify provider/getchu/getchu.go。

1. 保留注册名 Getchu 和优先级 1000。
2. NormalizeMovieID 使用 `(?i)^(?:GETCHU[-_])?([1-9][0-9]*)$`，非法输入返回空。
3. URL 仅接受 dl.getchu.com 的 /i/item<ID> 以及可选末尾斜杠；将归一化用于请求和 Homepage。
4. 初始化时设置重定向校验，保留共享 HTTP client 配置；404 映射 ErrInfoNotFound，独立 ErrChallenge 识别访问验证，未知/缺字段返回 ErrIncompleteMetadata。
5. 以 og:title/og:image 和当前作品资料表解析字段；时长只提取明确分钟字段 `(?:動画[：:]\s*|収録分数\s*約?\s*)([0-9]+)\s*分`，不能把 60 枚当 60 分钟。
6. Run: `go test ./provider/getchu -count=1`、`go vet ./provider/getchu`。

### Task 3: Gyutto 离线测试及实现

**Files:** Create provider/gyutto/gyutto.go、provider/gyutto/gyutto_test.go；Modify engine/register.go。

1. 增加注册名 Gyutto、优先级 1000，接口为 MovieProvider + MovieSearcher。
2. 测试数字 ID/GYUTTO-ID 归一化、规范 URL/推广 URL 归一化、恶意 URL/无效 ID 零请求拒绝。
3. GetMovieInfoByID 只按数字站内 ID 读取，不把 PNME-224 猜成 ID。
4. 详情在 #RightSide 限定容器解析；社团取 dd 首个链接，排除“サークル情報”；正文 Clone 后替换 br/p 保留段落；预览图去重。
5. 搜索用 japanese.EUCJP.NewEncoder().String(keyword) 编码后 url.Values.Encode，编码失败返回 ErrInvalidKeyword。采用完整 alt 标题，按 ID 去重，只接受合法作品 URL。未知页/损坏卡片不能报告为空结果；仅明确主搜索区空结果模板返回空切片。
6. 测试 EUC-JP 响应解码、日文/特殊字符搜索参数、正常/空/未知模板、广告链接、损坏卡片、外域重定向、404/验证/网络错误。
7. Run: `go test ./provider/gyutto -count=1`，先确认缺少实现导致失败，再实现并确认通过。

### Task 4: 引擎注册与验收

**Files:** Create engine/gyutto_test.go。

1. 断言按名称和 gyutto.com URL 找到相同 provider，推广 URL 可路由、伪造域名不可路由，不重复注册。
2. Run: `go test ./provider/getchu ./provider/gyutto ./provider/konodare ./engine ./route -count=1`。
3. Run: `go vet ./provider/getchu ./provider/gyutto ./engine ./route`、`go build ./...`、`git diff --check`。
4. 对给定详情 ID 做小范围在线验证。Getchu 曾出现本机代理 TLS EOF，若仍失败明确记录网络限制，不能当作测试通过或改成跳过校验。
5. 不自动 commit/push；提交只在用户明确要求时进行。

## 实施验收记录（2026-10-22）

- 给定样本经本机代理使用真实 Go provider 验证成功：Getchu 4058126、Gyutto 271421，均满足 MovieInfo.IsValid()，发行日 2024-06-28，时长 20 分钟，社团 ぷにもえ！，样本图链接各 3 条。
- Gyutto 搜索“ぬるテカ天国”返回 5 条并命中 271421；搜索 PNME-224 返回正常空切片，不进行外部番号推测。
- Getchu、Gyutto、Konodare、engine 回归测试通过；route 无测试文件。两站 provider 的 go vet、包含 engine/route 的静态检查以及 go build ./... 通过。
- `go test -race ./provider/getchu ./provider/gyutto ./engine -count=1` 通过，未报告数据竞态。
- 独立审查发现并修正三项问题，均先用新增测试复现：Gyutto 多个 .unit_DetailSummary 中必须按“作品概要”标题选择，不能取“スタッフ情報”；Getchu 仅有 OG 标签而无资料表的残页必须报 ErrIncompleteMetadata；两站最终 ID 必须与请求 ID 一致，跨 ID 重定向返回 ErrInfoNotFound。
- 审查修正后重新运行回归、race、go vet、go build 和两站 lint 均通过。再次在线验证两站作品简介均为 747 个字符，具有正确正文开头，不再误取工作人员姓名。
- ProviderID 遵循已有通用 Provider:ID 协议，无需修改 engine/providerid；Gyutto:271421 与 Gyutto:GYUTTO-271421 由现有规范化链路处理。
- API 使用：`GET /v1/movies/Getchu/4058126`、`GET /v1/movies/Gyutto/271421`；标题搜索使用 `/v1/movies/search?provider=Gyutto&q=<日文标题>`。客户端传 UTF-8 查询，provider 内部转换成 EUC-JP；不要让客户端再次手动转码。
- Getchu 没有 MovieSearcher，指定 Getchu 的搜索接口会沿用 engine 的按 ID 查询回退；Gyutto 有 MovieSearcher，因此站内数字 ID 详情应使用详情接口或以完整作品 URL 作为搜索 q，而不是假定网站关键词搜索会索引数字 ID。
- 两站 provider 的 golangci-lint 为 0 issues；扩大到 engine 时存在未改文件的 gci 格式问题，扩大到 engine/... 时另有旧 dockertest 依赖在 Windows 下的 SECURITY_DESCRIPTOR 类型检查错误。未为通过检查而修改这些无关代码。
- 本次联网只读取给定详情样本和小范围搜索 HTML；没有下载图片/作品。此前代理 TLS EOF 本轮未再影响最终样本验证，不能据此承诺所有部署地区永久可访问。
