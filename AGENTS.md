# AGENTS.md

This file provides guidance to AI agents working with the ImageFunnel repository.

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

## 关键约定

- **依赖注入：** 所有实现所需的依赖都应该显式注入而不是依赖自己尝试获取并尝试降级
- **快速失败：**　逻辑不能正常进行应直接报错中止流程而不是尝试容错继续流程，**禁止**出错只记录日志然后告诉调用者正常完成，**禁止**添加未要求的错误处理，忽略错误数据或跳过操作**必须**先获得用户明确允许。如果你认为错误不影响主进程，则应该使用fire-and-forget模式异步处理而不是同步等待结果，既然等待结果就说明它会影响主进程。
- **调试友好：**　错误捕获禁止直接忽略任何错误，要么进行有意义的处理，要么只忽略具体的错误类型，禁止忽略错误，如果确定不可能出错应该添加panic。
- **立即实现：**　立即实现所有用户要求的功能，不能偷懒用注释标记为以后实现

### 术语

- 配套文件：以图片文件名加上额外后缀的文件为配套文件，和图片 basename 相同但是后缀不同的文件**不是**配套文件，因为相同名称不同格式可能有多个图片，图片不能是另一个图片的配套文件。

### Go

- **错误处理：**：循环中各项不互相依赖的场景，使用 `util.ErrorsBuilder` 各自处理后返回合并的错误，而不是直接日志记录后跳过或中途停止。
- **资源释放：**：局部资源比如锁或文件，尽量使用单独方法或IIFE搭配defer确保释放
- **无 `Get` 前缀**: 查询方法直接使用大写名称，如 `Session()` 而非 `GetSession()`
- **`iter.Seq` / `iter.Seq2[T, error]`**: 使用迭代器模式减少数组分配
- **构造函数**: 使用 `New` 前缀，如需清理，将清理函数作为第二个返回值
- **依赖注入**: 构造函数应通过参数接受所有依赖，不得在内部自行 `New*()` 构建。调用者（如 `main.go`）负责组装依赖图。这适用于 handler、service、factory 等所有带构造函数的类型
- **事件发布模式**: 移除统一的 `EventBus` 接口，改为直接依赖 `pubsub.Topic[T]`。领域层发布领域对象（如 `*pairing.Request`、`*device.Device`），应用层订阅后通过 `DTOFactory` 转换为 DTO。事件类型在 `shared` 包中定义（如 `FileChangedEvent`、`MetadataUpdatedEvent`），或直接在领域层使用领域对象。`main.go` 负责创建所有 `pubsub.Topic` 实例并注入到各组件
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
- **Filter 传入 0 长度数组**: 所有的过滤条件字段（通常是切片类型）如果传入 0 长度数组，代表“所有对象都不匹配”（应执行过滤并且匹配结果为空），而不是“不筛选”。“不筛选”的语义通过不传递该字段（在 Go 结构体中为 `nil` 指针或 `nil` 切片）来表示。因此，在 FilterBuilder 及其相关逻辑中，应用 `v != nil` 判断是否应用该过滤条件，而不是 `len(v) > 0`。
- **筛选逻辑集中**: 内存筛选逻辑集中在领域层 `FilterBuilder`。应用层和基础设施层禁止自行实现筛选，只能通过事件级粗筛后委托领域 `FilterBuilder` 做最终筛选
- **ID 生成**: 新建资源的 ID 生成应由领域层（如 `domain/*/service.go` 或实体构造器）自行负责，禁止由应用层（`application`）或接口层计算并传递 ID
- **ID 编码不导出**: `encodeID` 和 `decodeID` 均为非导出函数，ID 的编解码是领域内部实现细节，外部任何层级不得直接调用。需要将 ID 翻译为路径或获取领域对象时，应通过领域 Service（如 `directory.Service.GetDirectory`）或 Repository 获取领域对象后从中读取属性。
- **仓库构造权**: 领域实体的原始构造函数（如 `New`）不应导出供包外调用。外部构造入口统一为 `FromRepository` 专用方法，该方法内部调用非导出的构造函数与 `encodeID` 生成 ID。只有仓库实现有权限构造领域对象。
- **仓库接口接收已解码值**: Repository 接口的方法参数应当使用已解码的路径值（如 `relPath`、`absPath`），而非编码的 `scalar.ID`。ID 解码由领域 Service 在调用 Repository 前完成。例如 `directory.Repository.Get(ctx, relPath)`、`note.Repository.Read(ctx, relPath)`。
- **通过仓库获取 ID**: 当外部代码需要领域实体的 ID 时，应通过仓库获取领域对象后调用 `.ID()`，不得自行编码。需要目录 ID 的组件（如 `ImageScanner`、`image.Handler`）应注入 `directory.Repository`，通过 `Get` 获取 `*Directory` 后取其 `ID()`，而非从路径字符串自行编码。
- **应用层职责**: 应用层仅负责编排业务流程和翻译参数（如将接口层传入的 `directoryID` 通过 `directory.Service.GetDirectory` 转为领域对象再获取 `RelPath()`），所有规整文件名、路径拼接、ID 生成、冲突校验等具体业务逻辑应在领域层内执行
- **仓库接口极简设计**: `Repository` 接口必须保持极简且高度专注于数据的核心持久化（如 CRUD）。禁止为了计算物理绝对路径或辅助其他层创建空对象等非持久化行为而在接口中强行添加辅助方法。任何物理基准路径（如 `rootDir`）等硬件细节应通过依赖注入传入对应的领域服务（`Service`）或相关组件内部，由其内部自行处理。
- **空实体回退机制内聚**: 当部分接口为了支持 GraphQL 等层级的非空约束（如 `NonNull!`）需要对缺失的数据进行“空实体”回退（Fallback）时，空实体的构造和物理绝对路径计算应属于领域层 Service 的内聚职责（通常由 Service 的私有方法如 `newEmpty` 统一创建）。应用层禁止获取物理路径属性后手动拼装实体，以避免职责边界外泄与属性构建不一致。
- **应用层方法归属**: Handler 中的方法应根据操作对象归属到正确的领域包。如图片查询/订阅/移动操作应放在 `application/image.Handler`，笔记列表查询应放在 `application/note.Handler`，而非全部堆在 `application/directory.Handler`。`Root` 通过嵌入所有 handler 自动提升方法，GraphQL resolver 无需修改
- **应用层方法命名规范**: 属于特定领域 Handler 的方法，在全局上绝对不应该重名（防止 `Root` 结构体在嵌入提升方法时发生重名选择器歧义）。例如，获取列表的方法不应命名为通用的 `List`，而应直接以返回类型的名词复数命名（如 `device.Handler` 中命名为 `Devices` 替代 `List`，`hook.Handler` 中命名为 `Hooks` 替代 `List`），其他方法也应避免全局冲突。

- **DTO 跨领域引用**: `shared/*DTO` 中不应嵌入其他领域的 DTO 字段（如 `SessionDTO.CurrentImage *ImageDTO`），这会迫使 DTO 工厂导入其他应用层包造成跨领域耦合。改为存储 `scalar.ID`（如 `CurrentImageID scalar.ID`），零值即表示该关联不存在，GraphQL 层通过自动生成的 resolver 按 ID 查询完整对象。参照 `SessionDTO.CurrentImageID` 与 `session.resolvers.go` 中的 `CurrentImage` resolver
- **DTO 构造与职责边界**: `DTOFactory` 是唯一负责将领域对象（或基础设施层数据）转换为 DTO 的组件。禁止由 `DTOFactory` 以外的层级或组件直接控制 DTO 字段的生成或传递额外参数。`DTOFactory` 的 `New` 方法应仅接收领域实体本身，其派生的上下文状态（如 `parentID`、`isRoot` 等）应在领域对象被构造时自动计算并内置于实体中，由 DTOFactory 自动读取以完成映射。这可以避免外部调用层（如 Handler 或事件订阅）重复计算或越权传递参数，从而防止 Bug 引入。
- **间接关联获取与内聚ID生成**: 对于非扫描得到的间接图片关联（如垃圾箱历史中的封面图 CoverImage），如果其对应的物理文件存在于特定子目录，接口层（Resolver）严禁在本地手动拼装非标 ID 或本地手造 `ImageDTO`。应当在领域服务中增加 `ImageByRelPath`（无 `Get` 前缀）查询方法，直接根据相对路径从仓库加载实体，让其内部自动通过领域模型内聚生成带有修改时间的安全 ID，最后通过 `DTOFactory` 转换为标准的 `ImageDTO`。对于文件不存在的情况，Resolver 应当使用 `apperror.IgnoreNotFound` 或 `apperror.IsNotFound` 优雅过滤错误并返回 `nil` 降级，避免直接使用底层的 `os.ErrNotExist`，以防阻塞上层列表查询。
- **全局 Node 接口实现**: 任何具有全局唯一 ID 且需被客户端独立查询或被前端 Apollo Client 识别以进行缓存同步的实体（如 `Image` 实体），必须在 GraphQL Schema 中声明 `implements Node`，同时在 `node` 查询 Resolver 中补充其 ID 前缀的解析分支（如 `img:` 分发给 `r.app.Image`），保证能够被 `node(id)` 正确查询返回。

### Vue / TypeScript

- **声明式优先**: 使用 `computed` 而非 `watch` 维护状态。
  - _示例_：在切换目录时重置并隔离搜索词。避免使用命令式的 `watch(() => id, () => query.value = "")`，而是定义一个包含 `{ id, query }` 的局部缓冲区 `ref`，并利用带有 `get` 和 `set` 的 `computed` 属性声明式地处理状态：
    ```typescript
    const queryBuffer = ref({ id, query: "" });
    const query = computed({
      get: () => (queryBuffer.value.id === id ? queryBuffer.value.query : ""),
      set: (val) => {
        queryBuffer.value = { id, query: val };
      },
    });
    ```
- **模板引用**: 使用 `useTemplateRef`（单个）或 `@/composables/useTemplateRefs`（数组）
- **Composable 参数**: 使用 `MaybeRefOrGetter` 除非有特殊理由
- **null vs undefined**: 返回值使用 `undefined`，参数接受 `null`
- **GraphQL 类型**: 直接使用 `@/graphql/generated` 生成的类型
- **加载状态**: 使用样式和动画，避免文本提示导致布局抖动
- **lodash 替代**: 使用 `es-toolkit`
- **组件复用原则**: 若新建资源表单与已有编辑表单的提交模式（如新建采用手动保存，编辑采用自动保存）或输入字段有明显差异，应创建独立的表单组件，避免过度复用引入复杂的条件分支与防御性逻辑。
- **缓存策略优先**: 对于已提供 GraphQL 查询的数据（如工作流数据），应直接查询并依赖 Apollo Client 缓存，（可配置 `cache-first` 策略避免重复查询）避免在组件内声明多余的本地响应式状态进行二次缓存。这可以规避组件复用但 Prop（如 ID）切换时，本地状态未及时清理导致显示/复制旧数据的问题。
- **通知机制**: 成功和出错通知优先使用全局通知组件（如 `useNotification`），避免通过临时修改按钮文本等零散状态做交互，或仅用 `console.error` 打印错误。对于 GraphQL 操作，参考下方「错误通知去重」规则。
- **Tailwind 样式规范**: 没有特殊理由，不得使用带有 `.5`（如 `gap-1.5`、`p-2.5` 等）的间距/尺寸值或非标准的 `X50` 色彩数值（如 `bg-primary-750`、`bg-primary-850` 等）。同时，不得在没有特殊理由的情况下使用任意值语法，尤其是用像素指定尺寸（如 `text-[10px]`、`w-[100px]` 等）。应该默认使用基本单位的整数倍数（如 `gap-2`、`p-3`、`bg-primary-700` 等），保持样式尺度的统一。
- **导航组件统一**: 前端中静态且直接的页面导航，应该统一并尽可能使用 `<RouterLink>` 组件，而不是通过 JS 编程式触发（`router.push`），以保留浏览器原生的超链接体验（如支持 Ctrl+点击 和右键在新标签页中打开）。
- **导航禁用规范**: 当需要禁用某个导航项时，不应在 `RouterLink` 与 `button/div` 之间切换标签类型，应保持 `<RouterLink>` 的语义，并通过 CSS 样式类（如 `pointer-events-none opacity-40 cursor-not-allowed`）从浏览器层面对其禁用。同时，已被 `pointer-events-none` 禁用的元素上不得添加额外的 `@click` 防御性事件阻止逻辑，避免过度防御。
- **Composable 加载状态规范**: Composable 内部不应负责管理和返回 `loading` 状态，避免“多此一举”。如果调用者关心加载状态，composable 应当允许传入可选的 `loadingCount?: Ref<number>` 参数；如果不传入，内部不应为此创建多余的内部状态监控。加载状态的计算（例如 `computed(() => loadingCount.value > 0)`）应全权交由调用侧组件本地管理。
- **优先使用项目已存在工具**：实现某些通用逻辑时，先检查项目是否已经存在对于工具，或者考虑是否应该先实现一个通用工具，比如事件监听项目已经提供了 useEventListeners，不应该自己再重复手动实现

### GraphQL

- **Fragment**: 使用 fragment 避免重复定义查询字段，命名不带 Fragment 后缀
- **Schema 文件**: 每个字段/类型一个文件，使用 `snake_case` 命名
- **类型扩展**: 根对象字段超出第一个时使用 `extend type`
- **Schema 变更后**: 运行 `pwsh scripts/generate-graphql.ps1`，然后更新 resolvers
- **废弃字段**: GraphQL 不允许删除字段，只能标记 `@deprecated`。如需从 DTO 移除对应字段，gqlgen 重新生成后会自动为该字段生成 resolver stub（无需手动添加 `@goField(forceResolver: true)`），在 resolver 中实现原有的计算逻辑以保持向后兼容，不可随意返回空值
- **自动 resolver**: 当 `@goModel` 绑定的 Go 结构体缺少 schema 中的某个字段时，gqlgen 会自动生成 resolver，多此一举添加 `@goField(forceResolver: true)` 是冗余操作
- **零拷贝 DTO 绑定**: 优先使用 `@goModel` 将 GraphQL 类型直接绑定到后端的 `shared.*DTO` 结构体（不仅限于 Subscription，包括常规 Query/Mutation 对应的类型），使得生成的 resolver 签名能够直接传递/返回 DTO 结构，避免在 resolver 中编写额外的映射循环与手工转换代码。对于在 DTO 结构中缺失但 Schema 要求的动态字段（如与当前 Session/Context 状态有关的 `isCurrent`），应交由自动生成的对应字段级 resolver 独立解析；而在 DTO 工厂构造时可以安全静态拼装的数据（如由 UserAgent 解析的 `Name` 属性），应直接在 DTO 结构与工厂中定义和生成。
- **通用文件删除订阅**: 由于文件被删除或移走后其修改时间与属性均不可读，订阅删除事件时应使用通用的、仅提供相对路径的订阅（如 `dirEntryDeleted`）而非特定类型且包含 ID 的删除订阅（如 `imageDeleted`），以便前端统一基于相对路径比对从列表/缓存中移除对应的实体。
- **字段文档**: 每个新增或修改的字段、输入参数、枚举值都必须使用 `"""` 或 `"` 添加说明文档，基于实际实现描述语义而非机械翻译名称。若发现定义与实际实现不一致，用 `TODO:` 标记并说明原因
- **输入包装**: Mutation 的参数应尽量封装进 `input: *Input!` 中，以提供更好的扩展性，并便于前端获取生成的命名类型。
- **Schema 拆分粒度**: 禁止在 Mutation 文件的定义中夹带非相关的 Connection/Edge 类型或者 Query 字段。每个 Mutation/Query 所涉及到的自定义业务类型必须放入 `graph/types/` 下，查询字段放入 `graph/queries/` 下，以遵循严格的 `snake_case` 独立拆分规范。
- **避免冗余 success 字段**: Payload 结构体中禁止定义 `success: Boolean!` 等类似的标识字段。GraphQL 应依赖自带的 Error 抛出机制表达执行失败，只有在正常成功时才返回响应，避免冗余状态字段带来的反模式开发。
- **错误通知去重**: Apollo Client 的全局 `ErrorLink`（`frontend/src/graphql/client.ts`）会自动捕获所有 GraphQL 和网络错误并通过 `showError` 显示。因此：
   - 调用方**禁止**在 `catch` 块中重复调用 `showError`/`showNotification (..., "error")`，否则会导致重复通知
   - 调用方**禁止**仅使用 `console.error` 吞掉错误，这会阻止用户看到错误
   - 若调用方需要自行处理错误（如本地状态、特殊降级），应以 `context: { suppressError: true }` 传递给 `mutate()` / `query()` 以抑制全局通知，避免双重提示

### Python

- 标注所有参数类型，尽量避免 Any
- 修改后使用 ./scripts/check-python.ps1 检查类型

### 通用

- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **ID**: 客户端不应尝试解析 ID，格式不固定
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (no external PR triage). See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default names. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context. See `docs/agents/domain.md`.
