# ADR 0003: Accept 头协商图片格式，移除 fmt 查询参数

## 状态
Accepted

## 背景
原有的 `/image` 接口通过查询参数 `fmt=avif|webp` 显式指定输出格式。这导致以下问题：
1. 所有涉及图片 URL 的 GraphQL 查询都必须传递 `$format` 变量，增加了前端复杂度
2. `raw=true` 参数返回原图时错误地设置 `Content-Type: image/webp`
3. 无法在「质量要求高且期望宽度超过源图」时直接返回原图（违反了 format 要求）
4. 缓存键包含显式格式，导致同一源图不同格式无法共享缓存条目

## 决定
移除 `fmt` 查询参数，改用标准 HTTP `Accept` 头进行内容协商：
- 浏览器原生发送 `Accept: image/avif, image/webp, image/*, */*;q=0.8`
- 服务端解析 Accept 头（支持 q-value 排序），按优先级选择 AVIF/WebP
- 当无需缩放（width=0 或 width≥源图宽度）且源图 MIME 在 Accept 范围内时，直接返回原图并设置正确 Content-Type
- `raw` 忽略 Accept，返回源文件并通过内容嗅探设置正确 MIME
- 所有响应添加 `Vary: Accept` 头
- 签名不再包含 format，旧签名 URL 失效（本地应用可接受停机）

> 注：上面三条随后被后续改动收窄——直出改为 302 到 raw URL（见「后续：w 盲签与
> 全尺寸诉求 302 到 raw」），`raw` 是唯一不设置 `Vary: Accept` 的响应；
> 「无需质量压缩」中的 `quality` 条件已随 quality 参数一并移除（见下节），
> `raw=true` 的实际形式是空值 `raw`。当前判定条件与签署格式见
> [ADR 0004](./0004-image-url-and-signature-format.md)。

## 后果
### 正面
- 简化前端：移除 `getPreferredFormat()`、`isImageFormatSupported()` 及所有 GraphQL `$format` 变量
- 符合 HTTP 语义：标准内容协商，CDN/缓存正确工作
- 修复 `raw=true` Content-Type 错误
- 支持无损原图直通场景

### 负面
- 部署时缓存未命中风暴（本地应用可接受）
- 旧签名 URL 失效（本地应用可接受）
- 需要正确解析 Accept 头（已实现 q-value 排序）

## 实现细节
- 新增 `internal/util/content_type.go`：共享读取器 MIME 检测，避免重复打开文件
- 新增 `internal/util/accept.go`：Accept 头解析与 q-value 排序
- 修改 `internal/interfaces/http/image_handler.go`：核心协商逻辑
- 修改 `internal/infrastructure/urlconv/signer.go`：签名移除 format
- 修改 `internal/application/image/url_signer.go`：移除 `WithFormat`
- 更新所有 GraphQL schema 移除 `$format`，重新生成代码
- 删除 `frontend/src/utils/image-format.ts` 及测试
- 移除 `frontend/src/graphql/formatLink.ts` 及 client 中引用

## 后续：quality 参数的移除

本 ADR 落地后，`quality` 参数暴露出同类问题并被移除（与 format 属同一次契约收窄）：

**背景**：AVIF 编码质量改由服务端固定 CRF 决定（见 ADR 0002）后，`quality` 对 AVIF 完全失效，
但对 WebP 与「是否直出原图」的判断仍然生效——同一参数喂两个编码器而只有一个听，
属过渡期残留而非设计。

**决定**：画质一律由服务端编码器配置决定，从 URL 契约中移除 `quality`：
- AVIF 由 `svtav1CRF` 决定；WebP 由 `magick.webpQuality` 决定（取值 92——实测相比 q75 画质高
  约 3.3dB 而编码仅慢约 15ms，且远离 q100 的无损编码悬崖：q100 时 1024w 编码从 501ms 涨到 2496ms）
- 「是否直出原图」仅由宽度判定（`width == 0 || width >= 源宽` 且源图 MIME 在 Accept 内）——
  转码与否本质是分辨率问题，与画质无关
- 签名不再包含 `q`；GraphQL `url` 字段只保留 `width` 入参
- `Spec` 不再包含 quality，变体缓存键随之少一个维度，同一档位不会因画质参数重复编码

> 注：此处原先另有一条「请求携带 `q` 时返回 400」。该显式白名单随后被
> [ADR 0004](./0004-image-url-and-signature-format.md) 移除——签名改为覆盖整段原始字符串后，
> 任何未参与生成的参数都会验签失败，逐个拒绝既冗余又易漏。签署内容的完整定义见 ADR 0004。

**代价**：WebP 失去运行时画质调节能力，调整画质需改配置并重新发版。评估为可接受——
通过前端查询传递画质本身也需要发版，因此并未真正失去灵活性。

## 后续：w 盲签与全尺寸诉求 302 到 raw

前两节遗留了一个只解决「返回什么字节」、未解决「同一张图被客户端缓存成几份」的问题：
签发侧按已知源宽决定是否签 `w`，`w` 的 URL 形状依赖服务端状态；而全尺寸诉求
（不带 `w` 或 `w >= 源宽`）会在 `/image` URL 下直接返回源图字节，与 `rawURL` 形成两个缓存条目。

**决定**：

- 签发侧盲签：`url(width)` 不再读取图片宽度。`width` 为空不签 `w`，`width > 0` 一律签，
  `width <= 0` 返回 GraphQL error。`w >= 源宽` 的 URL 因此从「resolver 不会生成」
  变为常规可达（例如 2304px 的图在高倍查看时请求 `url(width: 4096)`）。
- `w` 是需求而非状态：表示期望的最大宽度，服务端不会放大；各编码器均按
  `min(w, 源宽)` 收缩。`raw` 是唯一直接返回全尺寸字节的 URL。
- `/image` 请求路径（验签 403 → 读取参数 → 304 短路后）：
  - `raw`：一次打开源图、嗅探 MIME 后服务；只嗅探不读宽度（源图损坏/截断仍可直出字节），
    响应不带 `Vary: Accept`（内容与 Accept 无关）；
  - 不带 `w`：一次打开源图嗅探；源图 MIME 被接受则 302 到 raw URL，否则全尺寸转码（`w = 0`）；
  - `w > 0`：先只查变体缓存（仅 stat 缓存文件，不打开源图）；未命中才一次打开源图同时取宽度与 MIME；
    `w >= 源宽` 且源图 MIME 被接受则 302 到 raw URL，否则按 `min(w, 源宽)` 转码；
  - `w` 非数字、`0` 或负数返回 400，不再静默退化为原图；`raw` 与 `w` 同时出现返回 400。
- 宽度来源：标准库可解码的格式（JPEG/PNG/GIF/WebP）在同一句柄上 `DecodeConfig`；
  本地无解码器的格式（AVIF/BMP/TIFF）回退到既有的 `Processor.Meta`（magick），
  仅这类格式会多一次元数据读取——「单次 open」保证适用于标准库可解码格式。
- 302 形状：目标 raw URL 用请求自带的 `t`/`s` 重新签名（t/s 变化即不是同一张图，
  不得改写为读盘现状）；无 body，HEAD 同样；缓存头与 200 完全一致
  （`Cache-Control: public, max-age=31536000, immutable`、`ETag: "immutable"`），
  并保留 `Vary: Accept`（重定向决策依赖 Accept）。重验证由既有 `If-None-Match` 304 短路完成。
- 变体缓存键维持不变（源身份 + `w`/`min(w, 源宽)` + 输出格式 + 编码器），不投影 `Accept`。

**后果**：同一图片的全尺寸诉求在浏览器侧收敛到 raw URL 一个缓存条目；不带 `w` 且源图
MIME 可接受时同样重定向。非浏览器客户端在源图 MIME 不被接受时仍会按不同 `w` 的 URL
缓存相同字节（另见 issue #172）。
