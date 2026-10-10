# 18AV 播放器 ID 解密说明

> 站点:`https://18av.mm-cg.com`
> 记录时间:2026-10-10
> 状态:**算法已完整还原并实测复现**(3 个页面验证通过)
> 用途:如果后续需要取该站的完整片源(m3u8),可直接复用本文档,无需重新逆向。

---

## 1. 背景

18AV 的影片详情页本身**不含**任何可直接播放的视频地址。真正的播放器地址由一个客户端 ID 参数决定:

```
https://18av.mm-cg.com/js/player/play.php?numresolution=1080&id=<decrypted_token>
```

该 `id` 在详情页 HTML 中是**加密**的,由内联脚本 + `/js/content_mv_protect.js` 在浏览器端解密后写入 iframe。

`content_mv_protect.js` 经过「Dean Edwards packer 双层打包 + 标识符混淆 + CryptoJS 内联打包」处理(约 11.7 KB),直接阅读困难。但**解密入口函数在混淆后仍可读**,且算法本身很简单。

> ⚠️ `robots.txt` 显式 `Disallow: /js/content_mv_protect.js`,说明这确实是站方认定的保护资产。仅供个人媒体库研究用途。

---

## 2. 详情页中的密文来源

详情页内联脚本(位于 `<body>` 开头)形如:

```js
hadeedg252=10;hcdeedd252=10;var hdddedd252 = 'bb8e07b8b673b2e9';var argdeqweqweqwe = 'bb8e07b8b673b2e9';
hadeedd252=14;var hdddedg252 = '0bf9e260618b7ec6';var argdeqweqweqww = '0bf9e260618b7ec6';
hcdeedg252=14;hadeedf252=10;var argdeqweqweqwz = 'bb8e07b8b673b2e9';
hcdeedf252=14;var hdddedf252 = '0bf9e260618b7ec6';
```

以及 `mvarr` 中的密文(108 个以 `o` 分隔的 token,另有同密文的 decoy 副本):

```js
mvarr['10_1']=[[ 'a_iframe_id_...', '<108字符密文>', '<iframe前缀>',
                 '//18av.mm-cg.com/js/player/play.php?numresolution=1080&id=', '', '></iframe>...' ]];
mvarr['11_1']=[[ ... ]];   // 720p
```

### 2.1 关键常量

| 变量名 | 含义 | 示例值 |
|---|---|---|
| `hcdeedg252` | **进制 radix** | `14` |
| `hadeedg252` | **异或 key** | `10` |
| `argdeqweqweqwe` | **AES key**(16 字节 ASCII) | `bb8e07b8b673b2e9` |
| `argdeqweqweqww` | **AES IV**(16 字节 ASCII) | `0bf9e260618b7ec6` |

> ⚠️ **key / IV 每次请求都会变**,且密文也变(见 §5)。三者必须在**同一次 HTTP 响应**中一起解析,不可跨请求混用。
> `radix` / `xor` 实测恒为 `14` / `10`,但仍建议从页面解析而非硬编码。
> 页面上另有 `hdddedd252` / `hdddedg252` / `argdeqweqweqwz` / `hdddedf252` 等**同值诱饵变量**,不要被名字误导 —— 以上表为准。

---

## 3. 算法

### 步骤 1:混淆解码(自定义 base-14 → 字符)

```js
function decrypto(enc, radix, xorKey) {
  const sep = String.fromCharCode(radix + 97);      // radix=14 → 'o' (0x6F)
  return enc
    .split(sep)
    .map(t => String.fromCharCode(parseInt(t, radix) ^ xorKey))
    .join('');
}
```

- 分隔符 = `radix + 97`(14 + 97 = 111 = `'o'`)
- 每个 token 按 **radix 进制**解析(radix=14 时合法数字字符为 `0-9a-d`)
- 结果与 `xorKey`(10)**异或**,再转字符

输出是一个 **Base64 字符串**(80 字节密文,含 `=` 填充)。

### 步骤 2:AES-128-CBC + PKCS7 解密

```
明文 token = AES-CBC-PKCS7-Decrypt( Base64Decode(步骤1输出), key, iv )
```

- key / iv 均按 **UTF-8 直接取那 16 个 ASCII 字符的字节**(不是 hex 解码)
- 明文形如:`WzhCbIvM1PAl8DeQFjAswfiXgC8T3vVwnbA9BFf-25mg73dJfArEh2PsWrhB80Yl`(48 字符)

### 3.1 原始混淆源码(已解包,供比对)

```js
function decrypto(g){
  var f=[];
  hcdeedg252 = 25>=hcdeedg252 ? hcdeedg252 : hcdeedg252%25;   // radix, 上限 25
  var h = String.fromCharCode(hcdeedg252 + 97);                // 分隔符
  g = g.split(h);
  for(h=0; h<g.length; h++){
    var k = parseInt(g[h], hcdeedg252);
    k = 1*k ^ hadeedg252;
    k = String.fromCharCode(k);
    f.push(k);
  }
  return f.join("");
}

function decr_sun(g, f, h, k){
  g = decrypto(g);
  var n = CryptoJS.enc.Utf8.parse(argdeqweqweqwe);   // key
  var r = CryptoJS.enc.Utf8.parse(argdeqweqweqww);   // iv
  g = CryptoJS.AES.decrypt(g, n, {iv:r, padding:CryptoJS.pad.Pkcs7});
  $("#"+k).attr("src", f + g.toString(CryptoJS.enc.Utf8) + h);
}
```

> 注意:`CryptoJS.AES.decrypt` 传入**字符串**密文时,默认按 OpenSSL 格式解析(Base64),这正是步骤 1 输出必须是 Base64 的原因。

---

## 4. 参考实现(Node.js,已实测)

```js
const crypto = require('crypto'); // 仅此一个依赖,Go 侧对应 crypto/aes + crypto/cipher

function extractConsts(html) {
  // key / iv:内联块中的两个 16-hex 常量
  const hexes = [...html.matchAll(/var\s+argdeqweqweqw[a-z]\s*=\s*'([0-9a-f]{16})'/g)].map(m => m[1]);
  const mXor = html.match(/hadeedg252\s*=\s*(\d+)/);
  const mRad = html.match(/hcdeedg252\s*=\s*(\d+)/);
  // 1080p 密文
  const mEnc = html.match(/mvarr\['10_1'\]=\[\['[^']*','([0-9a-z]+)'/);
  return { key: hexes[0], iv: hexes[1], xor: +mXor[1], radix: +mRad[1], enc: mEnc[1] };
}

function decrypto(enc, radix, xorKey) {
  const sep = String.fromCharCode(radix + 97);
  return enc.split(sep)
            .map(t => String.fromCharCode(parseInt(t, radix) ^ xorKey))
            .join('');
}

function solvePlayToken(html) {
  const c   = extractConsts(html);
  const b64 = decrypto(c.enc, c.radix, c.xor);
  const d   = crypto.createDecipheriv('aes-128-cbc',
                Buffer.from(c.key, 'utf8'), Buffer.from(c.iv, 'utf8'));
  return Buffer.concat([d.update(Buffer.from(b64, 'base64')), d.final()]).toString('utf8');
}
```

### 4.1 实测样本(可用于回归验证)

| 页面 | key | iv | 解密结果 |
|---|---|---|---|
| NHDTB-817(第 1 次) | `bb8e07b8b673b2e9` | `0bf9e260618b7ec6` | `WzhCbIvM1PAl8DeQFjAswfiXgC8T3vVwnbA9BFf-25mg73dJfArEh2PsWrhB80Yl` |
| NHDTB-817(第 2 次) | `51c81a62e88976e8` | `58921a50c877106a` | 同上(**明文一致**) |
| START-343 | `6e3c1667831bdc72` | `50be4409f5a77dbb` | `-xBnBb0ykNrvi7X6p5YYMthF1SM7bW4Xuol193H0p0zEmo_YcrnFEfm_6aJmL-by` |

---

## 5. 端到端链路

```
① GET  详情页  /zh/{type}_content/{internalID}/{NUMBER}.html
        ↓ 解析 radix / xor / key / iv / enc
②      decrypto() → Base64 → AES-128-CBC 解密 → playToken
③ GET  /js/player/play.php?numresolution={1080|720}&id={playToken}
        (建议带 Referer = 详情页 URL)
        ↓ 返回 HTML,内含明文 videoSources 数组,直接正则即可
           const videoSources = [
             {src:'https://…/31/1/…/….m3u8', type:'application/x-mpegURL', size:720},
             {src:'https://…/31/2/…/….m3u8', type:'application/x-mpegURL', size:1080},
           ];
④ GET  m3u8 播放列表
⑤ 解密并拼接分片
```

### 5.1 m3u8 播放列表的坑

实测拿到(1080p):

```m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:9
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-KEY:METHOD=AES-128,URI="https://cdn-…b49a0959….streamfastpro.com/aavv.vv",IV=0xd9f1c97e8ec6c96376a7b5039a302e0f
#EXTINF:1.201200,
https://cdn-…b49aO959….streamfastpro.com/…/….js
#EXTINF:7.240567,
https://cdn-…b49aO959….streamfastpro.com/…/….js
…
#EXT-X-ENDLIST
```

三个必须处理的点:

| # | 现象 | 处理 |
|---|---|---|
| 1 | **分片域名与播放列表域名差一个字符**:playlist 是 `…b49a**0**959…`(数字零),分片是 `…b49a**O**959…`(字母 O) | 不能直接使用 playlist 里的分片 URL,**需把 host 中的 `0` 换成 `O`**(或反之,以实际可连为准) |
| 2 | 分片后缀伪装成 **`.js`** | 按 MPEG-TS 处理即可 |
| 3 | 分片 **AES-128 加密**,密钥 URI 为 `/aavv.vv`(可从 `#EXT-X-KEY` 行取);`IV` 也在该行 | 下载 key 后按 AES-128-CBC 解密分片 |

**完整片验证**:该 m3u8 含 **2214 个分片,总时长 13146.7s ≈ 219.1 分钟**,与详情页"影片時長: 219分"吻合 —— 确认是**完整正片**,而非预告片。

---

## 6. 注意事项

1. **key / IV / 密文每次请求都变**,但解密结果对该影片稳定 → 服务端是无状态随机化封装,客户端算法固定。
2. **play.php 只认解密后的 token**。直接传原始密文或内部数字 ID 都只返回 110 字节的空壳页面(实测)。
3. **`id` 可能有时效性**,建议「取页面 → 解密 → 立即请求 play.php」一气呵成,不要缓存 token 跨时段复用。
4. **播放器 token 与分辨率绑定**:`numresolution=1080` / `720` 都能取到,返回的 `videoSources` 里两个分辨率都在,实际由前端切换。
5. **域名/变量名可能变**:`hdd…252` / `argdeq…qwe` 这类名字是半随机生成的;稳妥做法是按**结构**匹配(`16 位 hex 常量`、`108 字符密文`),而非写死变量名。
6. **主域会漂移**:站内公告显示存在 `mm18vd.com` / `mm18news.com` 等发布页。

---

## 7. 其他相关规律(非解密,但配套)

```
搜索:   /zh/{searchType}_search/all/{keyword}/{page}.html
        searchType: fc / censoredonly / reducing-mosaiconly / amateurjavonly
                    / uncensoredonly / animationonly / chineseonly / dt / cg / cwp / novel
详情:   /zh/{type}_content/{internalID}/{NUMBER}.html   ← internalID 不可从番号推导,必须搜索获取
封面:   https://{fb|fc}host1.imgstream2.com/b/{category}/{internalID}_{NUMBER}.jpg
缩略图: 同上,/b/ → /s/
预览片: https://gifb.imgstream2.com/hc/9smv_{category}_{internalID}_{NUMBER}.mp4   (约 200KB 短预告)
演员页: /zh/{type}_avperformer/{actorID}/{name}/{page}.html   (仅影片列表,无简介/头像)
```

---

## 8. 复现/回归步骤

```bash
# 1. 取详情页(必须带 UA;走本机代理)
curl --proxy http://127.0.0.1:7890 -s --compressed \
     -A "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/131.0.0.0" \
     "https://18av.mm-cg.com/zh/censored_content/202186/NHDTB-817.html" -o movie.html

# 2. 运行 §4 的 Node 脚本,输出 playToken
# 3. 请求 play.php(带 Referer),正则抓 videoSources
# 4. 取 m3u8
```

> 站点不支援日本 / 美国 IP 浏览(站内公告),注意出口节点选择。

---

## 9. 与 metatube-sdk-go 的关系

`metatube-sdk-go` 的 `model.MovieInfo` **没有完整片源字段**;`PreviewVideoURL` / `PreviewVideoHLSURL` 语义是「预告片」。`provider.Fetcher` 经查仅被 `route/image.go` 用于**图片代理**。

因此本文档的成果**不能直接落进标准 provider**,仅在有独立播放/下载需求时使用。

元数据刮削(标题/番号/演员/片商/类型/日期/时长/封面)与本文档无关,走 HTML 解析即可 —— 详见详情页的 `ld+json` 与 `posts-inner-details-text` 信息块。
