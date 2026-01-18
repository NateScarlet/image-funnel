# ImageFunnel 开发指南

## 项目概览

### 核心概念

**ImageFunnel** 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

**核心特点：**

- **无侵入式元数据管理**：使用 XMP Sidecar 文件存储筛选结果，不修改原始图片
- **移动优先的 Web 界面**：支持手势操作和键盘快捷键
- **三态分类工作流**：保留/稍后再看/排除，避免决策疲劳
- **专业工具兼容**：Adobe Lightroom/Bridge、XnView 等可直接读取评分

### 技术栈

- **后端**：Go + gqlgen（高性能并发处理，实时 GraphQL 接口）
- **前端**：Vue 3 + TypeScript + Tailwind（快速开发响应式移动界面）
- **元数据**：XMP Sidecar 文件（遵循 Adobe 标准）
- **存储**：文件系统（零额外数据库）

### 核心功能模块

1. **目录与图片管理**
   - 支持格式：JPEG、PNG、WebP、AVIF
   - MVP 版本：仅处理根目录下的直接图片文件
   - 后期扩展：递归扫描、文件系统监控、图片去重

2. **评分映射系统**
   - 预设类型：草稿阶段筛选、细化阶段筛选、自定义预设
   - 内置默认预设，支持用户自定义
   - 队列开始时选择预设

3. **筛选工作流**
   - 初始化：选择目录、设置保留目标、选择预设
   - 筛选循环：显示图片、三按钮操作、进度跟踪
   - 完成阶段：显示摘要、确认写入 XMP 文件

4. **XMP Sidecar 实现**
   - 文件格式：标准 XMP RDF/XML
   - 核心字段：`xmp:Rating`（主评分 0-5）
   - 扩展字段：`imagefunnel:Action`、`imagefunnel:Timestamp`、`imagefunnel:Preset`
   - 写入策略：批量写入、原子操作、增量更新

### 关键设计决策

**元数据策略：**

- XMP Sidecar 优先，不修改原始图片
- 保护 AI 生成工具写入的元数据
- 零额外存储，仅依赖文件系统

**工作流设计：**

- 量化目标：设定保留数量目标
- 阶段化筛选：支持不同筛选阶段使用不同评分策略
- 可控提交：批量操作后确认再写入

## 环境配置

项目已配置好 VS Code 调试启动器，位于 `image-funnel.code-workspace`。

## 快速启动

直接要求用户按 F5 键启动调试器，不要尝试自己启动

## 开发工作流

### 修改 GraphQL schema 后

运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码

运行 `pnpm check` 来检查错误

### 修改前端代码后

运行 `pnpm check` 来检查错误

### 修改后端代码后

运行 `.\scripts\build.ps1` 来重新编译前端和后端

### 测试

- 访问 http://localhost:8080（前端）
- 访问 http://localhost:8000/graphql（GraphQL Playground）

## 项目结构

```
image-funnel/
|-- scripts/             # 构建脚本
├── cmd/server/          # 后端入口
├── frontend/            # 前端项目
├── graph/               # GraphQL schema 和 resolver
├── internal/            # 后端业务逻辑
└── data.local/          # 图片目录（默认）
```
