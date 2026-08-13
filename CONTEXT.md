# ImageFunnel 领域语言

图片筛选工作流——帮助用户从大量 AI 生成图片中通过三态分类快速筛选出优质图片。

## 核心概念

**筛选会话 / Screening Session**
针对某个目录的一次图片筛选工作单元。维护了待筛选队列、三态分类操作历史、撤销栈和提交能力。一个目录可创建多次独立的筛选会话。

**图片 / Image**
文件系统中的一个图片文件，带有 XMP 配套文件中存储的评分和标签。筛选过程的基本操作单元。
_避免_: Asset, media, resource

**目录 / Directory**
文件系统子目录在领域中的映射。组织成层次结构，根目录为整个筛选的入口。每个目录可持久化其浏览状态、最后会话引用等配置。

**三态分类 / Three-state Classification**
筛选的核心决策模型。每张图片可被标记为三种状态之一：
- **保留 / Keep** — 优质候选
- **搁置 / Shelve** — 暂时不确定
- **排除 / Reject** — 不合格
_避免_: Delete, archive, approve, reject-keep（没有第四种选项）

**标记 / Mark**
在会话中对一张图片应用三态分类决策的操作。每次标记自动推进队列到下一张图片。
_避免_: Rate, classify, assign（Mark 特指会话过程中的即时决策动作）

**提交 / Commit**
将会话中所有标记结果写入图片对应 XMP 配套文件的操作。提交后筛选结果在文件系统中固化。
_避免_: Save, export, flush

**撤销 / Undo**
撤销最近一次标记操作。支持跨轮次、跨筛选条件撤销。
_避免_: Rollback, revert, go-back

**轮次 / Round**
一轮完整的队列遍历。当前轮所有图片均被标记后，可对保留的图片开启新一轮筛选。新轮次自动打乱顺序并避免连续出现同一张图片。

**队列 / Queue**
当前轮次中待标记图片的有序列表。文件系统变更时可动态更新（新增图片加入、删除图片移除）。

## 操作类型

**筛选操作 / ImageAction**
三态分类的具体动作值：`KEEP`（保留）、`SHELVE`（搁置）、`REJECT`（排除）。

**文件变更动作 / FileAction**
文件系统上发生的事件类型：`CREATE`（新建）、`WRITE`（写入）、`REMOVE`（删除）、`RENAME`（重命名）。

## 配套实体

**配套文件**
以图片文件名加上额外后缀的文件为配套文件，和图片 basename 相同但是后缀不同的文件**不是**配套文件，因为相同名称不同格式可能有多个图片，图片不能是另一个图片的配套文件。

**XMP 配套文件 / XMP Sidecar**
路径为图片路径 + `.xmp` 的独立文件，存储该图片的评分、颜色标签和筛选操作。筛选结果的唯一持久化来源。原始图片文件永不修改。
_避免_: Metadata file, XMP file（需明确它是"配套文件"而非"元数据文件"，以避免与其它元数据源混淆）

**笔记 / Note**
以 Markdown 文件形式（`.md`）存储的自由文本。可关联到目录或图片，可选 YAML frontmatter 控制是否对用户可见（`hidden`/`hide`）。
_避免_: Comment, annotation（Note 是独立的 Markdown 文件，不是附属于 Image 的字段）

**目录状态 / Directory State**
持久化到目录下 JSON 文件的配置数据，包含：浏览过滤条件、最后活跃的筛选会话引用（lastSession 快照：filter + targetKeep）、以及默认的写入操作配置（各操作对应的 XMP 评分值）。
_避免_: Directory config, directory settings

**上次配置 / Last Configuration**
目录状态中可用于自动创建新会话的配置并集：`DirectoryState.lastSession` 快照（筛选条件 filter + 保留目标 targetKeep）与 `DirectoryState.default.writeActions`（写操作默认值）。用于首页「开始新筛选」时无弹窗自动复用，以及已完成会话自动切换到下一目录时复用。
_避免_: Last settings, previous config

**继续筛选 / Resume**
恢复目录的最后活跃会话（跳转到 `/session/{id}`），保留该会话进行中的队列、标记结果与撤销栈。
_避免_: Continue, reopen

**外部钩子 / Hook**
通过外部脚本扩展的可配置触发点。可在会话提交后自动触发，或由用户手动按图片/笔记触发。笔记还支持通过钩子提供 slash command 指令。

## 认证

**设备 / Device**
已注册的 WebAuthn 认证器设备。首次注册需通过配对请求人工授权。使用双令牌（访问令牌 + 刷新令牌）管理登录会话。

**配对请求 / Pairing Request**
新设备首次注册时创建的授权请求。通过验证码批准或拒绝，决定是否将该设备加入可信设备列表。
_避免_: Auth request, device request（Pairing 特指设备授权，不是用户登录）

## 隐藏概念

以下术语在代码中使用，但不直接暴露给用户：

**筛选条件 / Filter**
控制哪些图片进入筛选会话的条件集合。支持按目录、评分、标签、文件名查询筛选。所有条件间为 AND 关系。

**常驻补全 / Resident Autocomplete**
`[directive.autocomplete]` 配置中声明 `protocol = "json-rpc"` 后，应用按 command 维度维护常驻的 JSON-RPC 自动补全脚本进程：首次请求 spawn、后续请求复用、崩溃/不响应时自动重启、空闲回收并随应用退出统一回收。请求参数沿用现有自动补全上下文（`IMAGE_FUNNEL_AUTOCOMPLETE_*` + 图片/笔记上下文 + `rootDir`/`directoryRelPath`），响应复用现有 JSONL 建议结构。依赖由脚本最外层入口（serve 进程 / 单次 main）构建并注入，脚本核心不读取环境变量；依赖初始化失败快速失败（不降级）。取消通过 `$/cancelRequest` 通知（尽力而为），正确性由「丢弃过期响应 + 请求超时」兜底。未设置 `protocol` 的脚本保持单次执行行为（向后兼容，现有脚本零改动）。
_避免_: persistent process, daemon

**统计信息 / Stats**
会话粒度的统计（总数、保留数、搁置数、排除数、当前轮剩余数）和目录粒度的统计（图片数、子目录数、评分分布）。

## 项目概览

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

### 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **移动优先**: 深度优化的响应式 Web 界面，支持触摸手势
- **三态分类**: 保留/搁置/排除，避免决策疲劳
- **全功能撤销**: 支持跨轮、跨筛选条件的撤销操作
- **高性能**: 智能预加载、按需加载

### 技术栈

- **前端**: Vue 3 + TypeScript + Vite + Tailwind CSS 4
- **后端**: Go 1.24 + gqlgen + gorilla/mux
- **API**: GraphQL (Apollo Client + WebSocket subscriptions)
- **数据存储**: XMP sidecar 文件，无数据库

## 项目结构

```
image-funnel/
├── cmd/                 # 应用入口
│   └── server/
├── frontend/            # 前端项目
│   ├── src/
│   │   ├── components/  # Vue 组件
│   │   ├── composables/ # 可复用逻辑
│   │   ├── graphql/     # GraphQL 查询、变更、订阅
│   │   ├── views/       # 页面视图
│   │   └── utils/       # 工具函数
├── graph/               # GraphQL schema
│   ├── enums/
│   ├── mutations/
│   ├── queries/
│   ├── subscriptions/
│   └── types/
├── internal/            # 后端业务逻辑（六边形架构）
│   ├── domain/          # 核心业务逻辑，零外部依赖
│   │   ├── session/     # Session 聚合
│   │   ├── image/       # Image 实体
│   │   ├── directory/   # Directory 实体
│   │   ├── metadata/    # 元数据接口
│   │   └── note/        # Note 实体
│   ├── application/     # 应用层，业务层的简单封装
│   ├── infrastructure/  # 基础设施层
│   ├── interfaces/      # 接口层
│   │   ├── graphql/     # GraphQL resolvers
│   │   └── http/        # HTTP 路由
│   └── shared/          # 共享的无逻辑基础结构
└── scripts/             # 脚本
    ├── build.ps1        # 构建脚本
    └── generate-graphql.ps1 # 更新 GraphQL 相关代码
```

## 常用命令

```bash
# 前端
pnpm dev                 # 启动 Vite 开发服务器 (端口 8080)
pnpm build               # 生产环境构建
pnpm check               # oxlint 类型检查 + lint 自动修复 (修改前端后必须运行)
pnpm lint                # 仅 oxlint
pnpm lint:fix            # oxlint auto-fix
pnpm fmt                 # 使用 oxfmt 格式化代码

# 后端
go test --timeout 30s ./...      # 运行所有 Go 测试 (修改后端后必须运行)
go test --timeout 30s ./internal/domain/session  # 运行特定包的测试

# 构建与生成
pwsh scripts/build.ps1            # 完整构建 (前端 + Go，输出到 build/latest/)
pwsh scripts/run.ps1              # 开发模式 (同时运行前端和后端)
pwsh scripts/generate-graphql.ps1 # 重新生成 GraphQL 代码 (Go + TypeScript)
```

## 架构说明

后端遵循**六边形架构**（端口与适配器）：

1. **领域层 (domain)**: 核心业务逻辑，不依赖任何外部库
2. **应用层 (application)**: 编排领域层，提供用例
3. **基础设施层 (infrastructure)**: 技术实现（如内存存储、本地文件系统、XMP sidecar 等）
4. **接口层 (interfaces)**: 对外适配器（GraphQL、HTTP）

数据流程示例（标记图片）：

1. 用户操作 → Apollo Client 发送 `markImage` 变更
2. GraphQL resolver → `application/session.Handler.MarkImage()`
3. 应用层 → `domain/session.Service.MarkImage()`
4. Session 聚合更新队列、撤销栈、统计数据
5. 通过 `pubsub.Topic` 发布变更 → WebSocket 订阅 → 前端 UI 更新

