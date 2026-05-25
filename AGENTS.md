# AGENTS.md

This file provides guidance to AI agents working with the ImageFunnel repository.

## 项目概览

ImageFunnel 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

### 核心特性

- **XMP Sidecar 优先**: 不修改原始图片文件，通过独立的 XMP 文件存储筛选结果
- **专业工具兼容**: Adobe Lightroom/Bridge、XnView 等专业图片管理工具可直接读取评分
- **移动优先**: 深度优化的响应式 Web 界面，支持触摸手势
- **三态分类**: 保留(5星)/搁置(3星)/排除，避免决策疲劳
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
│   │   └── memo/        # Memo 实体
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
pnpm check               # TypeScript 类型检查 + ESLint (修改前端后必须运行)
pnpm lint                # 仅 ESLint

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

## 关键约定

### Go

- **无 `Get` 前缀**: 查询方法直接使用大写名称，如 `Session()` 而非 `GetSession()`
- **`iter.Seq` / `iter.Seq2[T, error]`**: 使用迭代器模式减少数组分配
- **构造函数**: 使用 `New` 前缀，如需清理，将清理函数作为第二个返回值
- **依赖注入**: 构造函数应通过参数接受所有依赖，不得在内部自行 `New*()` 构建。调用者（如 `main.go`）负责组装依赖图。这适用于 handler、service、factory 等所有带构造函数的类型
- **EventBus 接口本地化**: 当 handler 需要事件总线但引用 `application/session` 中的接口会造成循环导入时，在包内定义同名本地接口（如 `type EventBus interface { SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] }`）。Go 的隐式接口满足确保 `infrastructure/ebus.EventBus` 无需显式声明即可匹配。参照 `memo.Handler` 和 `image.Handler` 中的实践
- **编译时接口检查**: 使用 `var _ Interface = (*Impl)(nil)`
- **错误处理**: 绝不静默忽略错误，使用 `errors` 包处理
- **业务错误**: 使用 `internal/apperror` 包
- **枚举**: 放在 `internal/shared/enums.go`
- **日志**: 使用 zap，日志消息小写，记录耗时用 `duration` 字段，长耗时操作前后用 `will`/`did` 前缀
- **测试**: 新增功能时添加对应的单元测试，测试文件名与逻辑文件对应
- **Context**: 使用 `context.Context` 传递请求上下文
- **类型命名**：不要在命名中重复包名，除名称正好和包名相同
- **筛选器**: 每个领域实体统一使用 `FilterBuilder` struct + `Build` 方法模式（参考 `directory.FilterBuilder`），内部使用 `util.FilterBuilder` 和 `util.AddToSet` 构建筛选闭包
- **筛选器传值**: `Build` 方法接受值类型（非指针），调用方通过 `util.UnwrapPointer` 将可空指针安全转为零值
- **筛选逻辑集中**: 内存筛选逻辑集中在领域层 `FilterBuilder`。应用层和基础设施层禁止自行实现筛选，只能通过事件级粗筛后委托领域 `FilterBuilder` 做最终筛选
- **ID 生成**: 新建资源的 ID 生成应由领域层（如 `domain/*/service.go` 或实体构造器）自行负责，禁止由应用层（`application`）或接口层计算并传递 ID
- **ID 编码不导出**: `encodeID` 和 `decodeID` 均为非导出函数，ID 的编解码是领域内部实现细节，外部任何层级不得直接调用。需要将 ID 翻译为路径或获取领域对象时，应通过领域 Service（如 `directory.Service.GetDirectory`）或 Repository 获取领域对象后从中读取属性。
- **仓库构造权**: 领域实体的原始构造函数（如 `New`）不应导出供包外调用。外部构造入口统一为 `FromRepository` 专用方法，该方法内部调用非导出的构造函数与 `encodeID` 生成 ID。只有仓库实现有权限构造领域对象。
- **仓库接口接收已解码值**: Repository 接口的方法参数应当使用已解码的路径值（如 `relPath`、`absPath`），而非编码的 `scalar.ID`。ID 解码由领域 Service 在调用 Repository 前完成。例如 `directory.Repository.Get(ctx, relPath)`、`memo.Repository.Read(ctx, relPath)`。
- **通过仓库获取 ID**: 当外部代码需要领域实体的 ID 时，应通过仓库获取领域对象后调用 `.ID()`，不得自行编码。需要目录 ID 的组件（如 `ImageScanner`、`image.Handler`）应注入 `directory.Repository`，通过 `Get` 获取 `*Directory` 后取其 `ID()`，而非从路径字符串自行编码。
- **应用层职责**: 应用层仅负责编排业务流程和翻译参数（如将接口层传入的 `directoryID` 通过 `directory.Service.GetDirectory` 转为领域对象再获取 `RelPath()`），所有规整文件名、路径拼接、ID 生成、冲突校验等具体业务逻辑应在领域层内执行
- **应用层方法归属**: Handler 中的方法应根据操作对象归属到正确的领域包。如图片查询/订阅/移动操作应放在 `application/image.Handler`，备忘录列表查询应放在 `application/memo.Handler`，而非全部堆在 `application/directory.Handler`。`Root` 通过嵌入所有 handler 自动提升方法，GraphQL resolver 无需修改
- **DTO 跨领域引用**: `shared/*DTO` 中不应嵌入其他领域的 DTO 字段（如 `SessionDTO.CurrentImage *ImageDTO`），这会迫使 DTO 工厂导入其他应用层包造成跨领域耦合。改为存储 `scalar.ID`（如 `CurrentImageID scalar.ID`），零值即表示该关联不存在，GraphQL 层通过自动生成的 resolver 按 ID 查询完整对象。参照 `SessionDTO.CurrentImageID` 与 `session.resolvers.go` 中的 `CurrentImage` resolver


### Vue / TypeScript

- **声明式优先**: 使用 `computed` 而非 `watch` 维护状态
- **模板引用**: 使用 `useTemplateRef`（单个）或 `@/composables/useTemplateRefs`（数组）
- **Composable 参数**: 使用 `MaybeRefOrGetter` 除非有特殊理由
- **null vs undefined**: 返回值使用 `undefined`，参数接受 `null`
- **GraphQL 类型**: 直接使用 `@/graphql/generated` 生成的类型
- **加载状态**: 使用样式和动画，避免文本提示导致布局抖动
- **lodash 替代**: 使用 `es-toolkit`
- **`useStorage`**: 用于 localStorage，键格式为 `name@randomSuffix`，在 `<script lang="ts">` 块中定义以共享状态
- **组件复用原则**: 若新建资源表单与已有编辑表单的提交模式（如新建采用手动保存，编辑采用自动保存）或输入字段有明显差异，应创建独立的表单组件，避免过度复用引入复杂的条件分支与防御性逻辑。

### GraphQL

- **Fragment**: 使用 fragment 避免重复定义查询字段，命名不带 Fragment 后缀
- **Schema 文件**: 每个字段/类型一个文件，使用 `snake_case` 命名
- **类型扩展**: 根对象字段超出第一个时使用 `extend type`
- **Schema 变更后**: 运行 `pwsh scripts/generate-graphql.ps1`，然后更新 resolvers
- **废弃字段**: GraphQL 不允许删除字段，只能标记 `@deprecated`。如需从 DTO 移除对应字段，gqlgen 重新生成后会自动为该字段生成 resolver stub（无需手动添加 `@goField(forceResolver: true)`），在 resolver 中实现原有的计算逻辑以保持向后兼容，不可随意返回空值
- **自动 resolver**: 当 `@goModel` 绑定的 Go 结构体缺少 schema 中的某个字段时，gqlgen 会自动生成 resolver，多此一举添加 `@goField(forceResolver: true)` 是冗余操作
- **字段文档**: 每个新增或修改的字段、输入参数、枚举值都必须使用 `"""` 或 `"` 添加说明文档，基于实际实现描述语义而非机械翻译名称。若发现定义与实际实现不一致，用 `TODO:` 标记并说明原因
- **输入包装**: Mutation 的参数应尽量封装进 `input: *Input!` 中，以提供更好的扩展性，并便于前端获取生成的命名类型。

### 通用

- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **ID**: 客户端不应尝试解析 ID，格式不固定
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令
