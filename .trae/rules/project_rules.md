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

## 项目结构

```
image-funnel/
├── scripts/             # 脚本
│   ├── build.ps1        # 构建脚本，用于构建整个项目
│   └── generate-graphql.ps1 # 更新前后端的 GraphQL 相关代码
├── frontend/            # 前端项目
├── graph/               # GraphQL schema
├── internal/            # 后端业务逻辑
│   ├── interfaces/      # 接口层
│   ├── domain/          # 业务逻辑层
│   ├── application/     # 应用层，应该是业务层的简单封装
│   ├── infrastructure/  # 基础设施层，如数据库、文件系统等，按科技划分子包
│   └── shared/          # 共享的无逻辑基础结构和接口，所有层都可直接导入这里的包，并且这个包不导入任何层的代码
└── data.local/          # 开发测试使用的根目录，包含一些测试图片
```

## 注意事项

- id 不承诺固定格式， 客户端不应该尝试解析 id
- 代码逻辑块之间添加对理解上下文有帮助的注释，使用中文，避免简单翻译代码本身
- 长段关联的代码　用 vscode的 region comment （例如　`// #region {分组名称}` `// #endregion` ）包裹
- 不要手动修改生成的代码，而是用对应的脚本重新生成
- **frontend:** 修改前端代码后，使用 `pnpm check` 检查，详见 frontend-check SKILL
- **powershell:** 脚本用当前 shell 直接运行 (直接 "./scripts/xxx.ps1")，不要额外调用 `pwsh` 或 `powershell.exe`
- **go:** 修改代码后，运行包测试并使用 `scripts/build.ps1` 构建，详见 backend-build SKILL
- **go:** 所有测试必须带上合理的超时，防止死锁
- **go:** 用 errors 包处理错误，避免直接比较
- **vue:** 禁止使用watch更新ref的模式来处理数据变化，这种场景应该定义本地状态ref 和 computed，通过用 writable computed 更新本地状态，获取受数据影响后的本地状态