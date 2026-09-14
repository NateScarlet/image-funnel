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
- 当无需缩放（width=0 或 width≥源图宽度）且无需质量压缩（quality=0 或 quality≥95）且源图 MIME 在 Accept 范围内时，直接返回原图并设置正确 Content-Type
- `raw=true` 忽略 Accept，返回源文件并通过内容嗅探设置正确 MIME
- 所有响应添加 `Vary: Accept` 头
- 签名不再包含 format，旧签名 URL 失效（本地应用可接受停机）

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