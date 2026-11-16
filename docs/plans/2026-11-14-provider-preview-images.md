# 自增 Provider 剧照核实与适配结果

核实日期：2026-11-14。按 18AV → JavTrailers → NJAVTV → JAVXXX → AVWiki → Konodare → AirAVCity → GGJAV 的顺序施工。

## 结论

本轮新增 **JavTrailers** 的独立剧照解析，增强 **GGJAV** 的剧照解析并修复真实新版详情模板的必要元数据解析。原有 **Gyutto、AvGood** 剧照解析保留并回归测试。

因此，10 个自行新增的 Provider 中，当前有 4 个具备独立剧照解析实现：JavTrailers、GGJAV、Gyutto、AvGood。另 6 个没有可核实且已接入的独立剧照来源，不能标记为支持。以下判断仅针对核实样本，不代表站点所有历史模板都不存在剧照。

`MovieInfo.IsValid()` 不检查 `PreviewImages`；详情测试通过并不意味着剧照受支持。本轮测试对支持项明确断言剧照列表，对没有可靠来源的项明确断言不误采。

## 逐项处理

| Provider | 本轮结果 | 核实证据与处理原则 |
| --- | --- | --- |
| 18AV | 未新增剧照解析；补离线防误采测试 | `NHDTB-817`、`MAAN-1160` 详情模板只有播放器海报、演员图片、广告和推荐卡片，没有核实到独立剧照列表。封面与预告视频保持原行为，不由图片路径猜剧照。 |
| JavTrailers | **新增剧照解析** | SSR DOM 没有剧照图片标签；`script#__NUXT_DATA__[type="application/json"]` 的 Nuxt/devalue 引用表包含当前影片的 `gallery`。只沿 `data → video-{contentId} → video → gallery` 读取，并核对影片 `contentId`，不遍历推荐影片、演员或其他 URL 字符串。 |
| NJAVTV | 未新增剧照解析；补精灵图隔离测试 | 两个样本的播放器 `thumbnail` 提供 `col: 6, row: 6, width: 300, height: 168` 的 `seek/_N.jpg` 图片集合，属于进度条多格精灵图，不是独立剧照。不把它们直接写入 `PreviewImages`，也不下载、裁切或重新发布。 |
| JAVXXX | 未新增剧照解析；补空场景/推荐图隔离测试 | 两个核实样本的 `#scenes` 均为空；卡片的 `data-mediabook` 是 hover 预览资源，不能作为当前影片的剧照列表。没有据此推导额外 API 或文件编号。 |
| AVWiki | 未新增剧照解析；补封面/推荐图隔离测试 | `hhsi-024`、`hhsi-048` 的当前作品图片是 header 封面和 product 侧栏封面；正文 `.col_4` 图片属于其他作品推荐。不能泛抓 `.article-body img`。 |
| Konodare | 未新增剧照解析；补新旧模板隔离测试 | 新站 COSH-308 的 `.mdb-cosplay-image-link` 和旧站 PNME-333 的 `.cosplay-img img` 指向角色参考缩略图。实取一个 `_sc.jpg` 确认为 100×100 图片，并非独立剧照集合。不能只因具有 lightbox 链接便宣称支持影片剧照。 |
| AirAVCity | **无法完成实站剧照核实**；补已有模板空剧照断言 | 主站与已知镜像均返回 HTTP 403/Cloudflare challenge。保留现有元数据解析，不编造 gallery selector，不把推荐封面或由 `hid` 推导的地址当作剧照。离线测试不代表这次实站访问成功。 |
| GGJAV | **增强已有剧照解析** | 精确匹配 `div.previews` 类名 token 和当前影片 `/media/preview/{id}_{n}` 路径；兼容 `data-src`、`data-original`、`data-lazy-src`、`src`，保序去重并过滤非 HTTP(S)、占位、其他影片及非图片资源。真实当前模板使用原生 `loading="lazy"` + `src`；额外 data 属性兼容由离线用例覆盖。 |

### GGJAV 真实模板回归修复

真实页面复核发现，旧代码只读取 `div.title_text`，当前页面已经使用 `h2.title_text`；番号也由同一个文本节点改为 `.meta_label` 与 `.video_number` 两个兄弟节点。这会导致即使取到剧照，返回的影片元数据仍无效。

本轮同时兼容新旧标题节点、读取独立 `video_number`，并防止空的「番号：」标签覆盖已解析番号。新版模板测试还保证不会把后面的「プレビュー」或「その他の関連動画」标题当作影片标题。

## URL 与异常处理

- 共用 `provider/internal/previewimage` 工具仅规范化调用方明确识别出的剧照 URL，不负责泛抓页面图片或生成地址。
- 解析相对地址和协议相对地址；仅保留 HTTP(S)、非空 host、无用户凭证且具有常见图片后缀的地址；去掉 fragment 后保序去重，保留 query。
- Nuxt 引用表校验节点范围和影片身份；支持 Reactive/Ref 等包装，限制循环引用遍历。不执行脚本。
- 缺失、空或损坏的可选 gallery 不影响其他元数据解析。
- 不发起额外图片下载，不修改演员、广告、视频资源字段来伪造剧照支持；不将已有封面或预告片数量算作剧照数量。

## 实站页面与捕获回放

核实访问入口：

- [18AV NHDTB-817](https://18av.mm-cg.com/ja/censored_content/202186/NHDTB-817.html)、[18AV MAAN-1160](https://18av.mm-cg.com/ja/amateurjav_content/331742/MAAN-1160.html)
- [JavTrailers SSIS-001](https://javtrailers.com/ja/video/ssis00001)、[JavTrailers SVDVD-907](https://javtrailers.com/ja/video/1svdvd00907)
- [NJAVTV SSIS-001](https://njavtv.com/ja/ssis-001)
- [JAVXXX SSIS-001](https://www.javxxx.me/ja/video/ssis-001/)
- [AVWiki HHSI-024](https://av-wiki.net/hhsi-024/)、[AVWiki HHSI-048](https://av-wiki.net/hhsi-048/)
- [Konodare 新站 COSH-308](https://konodare.jp/cosh-308/)、[Konodare 旧站 PNME-333](https://konodare.wew.jp/pnme-333/)
- [AirAVCity 主站](https://airavcity4.cc/video?hid=99-21-14639)、[AirAVCity 镜像](https://airav.io/video?hid=99-21-14639)（本次均受 challenge 阻挡）
- [GGJAV 301794](https://ggjav.com/ja/main/video?id=301794)

对实际 HTTP 200 页面进行本地 mock transport 回放，而不是只检验人工 fixture：

| 页面 | 元数据有效 | 提取到的独立剧照 URL |
| --- | --- | --- |
| JavTrailers SSIS-001 | 是 | 10 |
| JavTrailers SVDVD-907 | 是 | 20 |
| GGJAV 301794 / KTRA-746 | 是 | 10 |

以上数量是页面声明并经改后解析器提取的 URL 数量，不是对所有图片 CDN 可达性、下载成功率或所有影片覆盖率的保证。尤其不擅自重写页面历史 CDN 域名。

## 回归验证

```bash
GITHUB_ACTIONS=true go test \
  ./provider/18av ./provider/javtrailers ./provider/njavtv ./provider/javxxx \
  ./provider/avwiki ./provider/konodare ./provider/airavcity ./provider/ggjav \
  ./provider/gyutto ./provider/avgood ./provider/getchu ./provider/fc2db \
  ./provider/internal/previewimage ./engine ./engine/providerid \
  -count=1 -timeout=120s

go build ./...
go vet ./provider/18av ./provider/javtrailers ./provider/njavtv \
  ./provider/javxxx ./provider/avwiki ./provider/konodare \
  ./provider/airavcity ./provider/ggjav ./provider/internal/previewimage
git diff --check
```

最终相关包回归、全仓构建、上述静态检查及差异检查均退出 0。真实页面回放的 10 / 20 / 10 张剧照与必要元数据断言也全部通过。

`GITHUB_ACTIONS=true` 使原有 testkit 跳过其外网测试。新增测试均使用本地 transport，不依赖站点、代理、浏览器或私密凭证。未运行需要 Docker/Postgres 的全仓数据库集成测试，也不把这份结果表述为全仓测试通过。

源码变更保留在本地，未推送远端。上游合并提交保持为 `473f2117654a6ea3398129f22fe1d5c7793d0305`。
