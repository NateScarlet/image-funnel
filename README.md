# ImageFunnel

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流（保留/搁置/排除）帮助用户从大量生成结果中快速筛选出优质图片。

## 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **移动优先**: 深度优化的响应式 Web 界面，支持触摸手势（缩放、平移）、移动端菜单，像原生 App 一样流畅
- **智能工作流**:
  - **三态分类**: 明确的 保留/搁置/排除 决策，避免决策疲劳
  - **全功能撤销**: 支持跨轮、跨筛选条件的撤销操作，不用担心手误
  - **目录导航**: 完成后自动切换到列表中的下一个目录
- **高性能**:
  - **智能预加载**: 根据当前进度预加载后续图片，秒开无等待
  - **按需加载**: 根据屏幕分辨率和缩放级别动态请求图片，节省带宽
- **安全可靠**:
  - **无损操作**: 始终保护原始图片，避免影响 Stable Diffusion 等工具写入的元数据
  - **原子写入**: 确保 XMP 文件写入不中断、不损坏

## 快速开始

### Windows 便携版

会自动检测安装所需依赖（ImageMagick），解压即用。

1. 下载最新版：[image-funnel-windows-x64.zip](https://github.com/NateScarlet/image-funnel/releases/latest/download/image-funnel-windows-x64.zip)
2. 解压到任意目录
3. 双击运行 `启动.cmd`

### 使用 Docker

监听目录变化的功能可能不起作用

1. 获取 [deployments/compose.yml](deployments/compose.yml)。
2. 将图片放到 ./images 下，或修改 /app/workspace 的挂载。
3. 运行 `docker compose up -d`。
4. 访问 `http://localhost:34898`。

### 从源代码构建

1. 安装依赖：
   - pnpm
   - go 1.24+
   - ImageMagick (推荐 v7+, 需添加到 PATH)
2. 构建并运行：
   - 调试运行: `scripts/run.ps1`
   - 编译构建: `scripts/build.ps1` (产物位于 dist 目录)

### 配置说明

可以通过环境变量调整应用行为：

- `IMAGE_FUNNEL_ROOT_DIR`: 待筛选图片的根目录。
- `IMAGE_FUNNEL_PORT`: 服务器监听端口 (默认 34898)。
- `IMAGE_FUNNEL_SECRET_KEY`: 用于签名 URL 的密钥。若不提供，将自动生成（重启后失效，建议生产环境固定）。

## 使用指南

1. 打开应用（浏览器访问 `http://localhost:34898`）
2. 选择根目录下包含图片的目录
3. 开始筛选：
   - **保留 (Keep)**: 选中 5 星
   - **搁置 (Shelve)**: 对应 "稍后再看"，3 星，不参与当前会话后续统计，直至提交
   - **排除 (Reject)**: 标记为 "排除"
4. 完成会话：
   - 点击右上角提交按钮
   - 确认写入操作（可自定义每个分类对应的评分）
   - 结果将保存到同名 `.xmp` 文件中
