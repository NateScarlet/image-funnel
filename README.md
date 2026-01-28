# ImageFunnel

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流（保留/搁置/排除）帮助用户从大量生成结果中快速筛选出优质图片。

## 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **AI 元数据保护**: 避免影响 AI 生成工具（如 Stable Diffusion）写入的元数据
- **移动优先**: 响应式 Web 界面，支持移动端操作
- **三态分类**: 明确的保留/待定/排除决策，避免决策疲劳

## 快速开始

### 使用 Docker

这是最简单的安装方式，包含了环境所需的所有依赖。

但是监听目录变化的功能可能不起作用

1. 获取 [deployments/compose.yml](deployments/compose.yml)。
2. 将图片放到 ./images 下，或修改 /app/workspace 的挂载。
3. 运行 `docker compose up -d`。
4. 访问 `http://localhost:34898`。

### 从源代码构建

1. 安装依赖：
   - pnpm
   - go
   - imagemagick

2. 在 windows 上运行[scripts/run.ps1](scripts/run.ps1) 即可。

### 配置说明

可以通过环境变量调整应用行为：

- `IMAGE_FUNNEL_ROOT_DIR`: 待筛选图片的根目录。
- `IMAGE_FUNNEL_PORT`: 服务器监听端口。
- `IMAGE_FUNNEL_SECRET_KEY`: 用于签名 URL 的密钥。若不提供，将尝试自动生成或使用随机密钥。

## 使用指南

1. 在浏览器中打开 `http://localhost:34898`
2. 选择或输入包含图片的目录
3. 开始筛选：
   - 使用鼠标点击、屏幕滑动或快捷键进行决策。
   - 决策结果会保存在当前会话中，并在完成后一键提交持久化到 XMP Sidecar 文件。
