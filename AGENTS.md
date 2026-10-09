# AGENTS.md

了解项目背景和上下文请查看 `CONTEXT.md`（以及 `CONTEXT-MAP.md`）；用户澄清后**必须**更新对应的 `CONTEXT.md` 以持久化上下文。

- **CONTEXT 边界**：`CONTEXT.md` 只承载统一语言，不放实现细节。判断标准：改动实现时若必须同步修改它，那这段内容就属于实现细节，应改放到代码注释、ADR 或本文 / [CODING_STANDARDS](CODING_STANDARDS.md)。助手开发指南（项目结构、技术栈、架构说明、常用命令）写在本文，不写进 `CONTEXT.md`。
- **立即实现：**　立即实现所有用户要求的功能，不能偷懒用注释标记为以后实现
- **完整重构：**　内部接口更改应修改所有调用者使用新的接口，不得以向下兼容为由保留旧的接口，
- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令
- **临时产物**: 临时产物放在 `.scratch`，不主动清理

## 项目结构

```
image-funnel/
├── cmd/                 # 应用入口
│   └── server/
├── frontend/            # 前端项目
│   └── src/
│       ├── components/  # Vue 组件
│       ├── composables/ # 可复用逻辑
│       ├── graphql/     # GraphQL 查询、变更、订阅
│       ├── views/       # 页面视图
│       └── utils/       # 工具函数
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

## 架构

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

各层的具体职责约束见 [CODING_STANDARDS](CODING_STANDARDS.md)。

## 技术栈

- **前端**: Vue 3 + TypeScript + Vite + Tailwind CSS 4
- **后端**: Go 1.24 + gqlgen + gorilla/mux
- **API**: GraphQL (Apollo Client + WebSocket subscriptions)
- **数据存储**: XMP sidecar 文件，无数据库

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
pwsh scripts/test.ps1            # 运行全部测试 (Go + Python + 前端，已适配受限沙箱)
go test --timeout 120s ./...     # 仅运行 Go 测试；受限沙箱内需先设置 GOCACHE=.scratch\go-build
go test --timeout 120s ./internal/domain/session  # 运行特定包的测试

# Python（example_hooks）
pwsh scripts/check-python.ps1    # unittest + pyright + black，统一使用根目录 .venv 开发环境
uv run example_hooks/runner.py   # hook 部署运行方式：依赖由各脚本头部 PEP 723 inline metadata 声明，
                                 # 经 uv run 提供；开发环境（根 .venv）与部署运行互不影响

# 前端测试
pnpm -C frontend exec vitest run # 仅运行前端测试（net use 沙箱兼容内置于 vite.config.mts，scripts 均已带 --configLoader native）

# 构建与生成
pwsh scripts/build.ps1            # 完整构建 (前端 + Go，输出到 build/latest/)
pwsh scripts/run.ps1              # 开发模式 (同时运行前端和后端)
pwsh scripts/generate-graphql.ps1 # 重新生成 GraphQL 代码 (Go + TypeScript)
```

## 修改或提交代码

在执行任何代码修改前，第一个 tool call 必须是阅读 [CODING_STANDARDS](CODING_STANDARDS.md)。

## Example Hook 包开发

使用 `scripts/check-python.ps1` 运行测试，不要尝试让测试支持直接运行或用框架运行。

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues (no external PR triage). See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default names. See `docs/agents/triage-labels.md`.

### Domain docs

Multi-context ("app" and "example_hooks"). See `docs/agents/domain.md`.
