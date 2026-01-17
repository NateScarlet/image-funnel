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
   - 扩展字段：`imagefunnel:Action`、`imagefunnel:SessionID`、`imagefunnel:Timestamp`
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

### 开发注意事项

#### 前端

尽量基于 ./graphql/generated 中的类型定义，不要自己定义重复的类型

避免使用 watch, 尽量使用 computed 进行数据转换和计算。

不要手动更新 graphql 查询结果，而是依赖 ./src/graphql/useQuery.ts 的响应式系统自动更新。InMemoryCache会自动更新查询结果。

#### 后端

所有字段没有特别理由，都不应该导出，只能通过方法访问。
getter 应该处理 nil 值，返回默认值或空字符串等。避免给getter添加`Get`前缀。
setter 应该验证输入值的有效性，避免无效状态。
构建函数使用 NewXXX 风格命名，校验参数是否有效，参数顺序与字段顺序一致。

使用Options模式来指定命名参数。命名参数的名称以 `{函数名称}With` 开头，后面跟着参数名的驼峰式命名。

使用领域驱动设计（DDD）架构，将业务逻辑与数据访问分离。

测试出错时，在测试中添加详细的日志输出，帮助定位问题。

编写测试时可以用 github.com/stretchr/testify/assert 或 require 来验证结果是否符合预期。

不要用 go run 编写测试，直接用 go test 运行测试。

## 环境配置

项目已配置好 VS Code 调试启动器，位于 `image-funnel.code-workspace`。

## 快速启动

直接要求用户按 F5 键启动调试器，不要尝试自己启动

## 开发工作流

1. **修改代码后：**
   - 后端：调试器会自动重新编译（如使用 `dlv`）
   - 前端：Vite 会自动热重载

2. **修改 GraphQL schema 后：**
   - 运行 `.\scripts\generate-graphql.ps1` 命令来同时更新前后端的 GraphQL 相关代码
   - 运行 `pnpm run check` 来检查错误

3. **修改前端代码后：**
   - 运行 `pnpm run check` 来检查错误

4. **修改后端代码后：**
   - 添加必要的测试用例
   - 运行 `go test --timeout 30s` 测试修改的模块，如果大量修改　直接运行 `go test --timeout 600s ./...` 测试所有模块
   - 运行 `.\scripts\build.ps1` 来重新编译前端和后端

5. **测试：**
   - 访问 http://localhost:3000（前端）
   - 访问 http://localhost:8080（GraphQL Playground）

## 项目结构

```
image-funnel/
|-- scripts/             # 构建脚本
├── cmd/server/          # 后端入口
├── frontend/            # 前端项目
│   └── src/
│       ├── components/  # Vue 组件
│       ├── graphql/    # GraphQL 客户端
│       │   ├── fragments/    # GraphQL 片段
│       │   ├── mutations/    # GraphQL 变更操作
│       │   ├── queries/      # GraphQL 查询
│       │   ├── subscriptions/# GraphQL 订阅
│       │   ├── client.ts     # GraphQL 客户端配置
│       │   └── generated.ts  # 自动生成的类型
│       └── views/       # 页面视图
├── graph/               # GraphQL schema 和 resolver
│   ├── schema.graphql  # 主 GraphQL schema 定义
│   ├── models_gen.go   # gqlgen 自动生成的模型
│   ├── resolver.go     # 主 resolver 入口
│   ├── scalars.go      # 自定义标量类型（Time、Upload、URI）
│   ├── *.resolvers.go  # 各 mutation/query 的 resolver 实现
│   └── mutations/      # Mutation 定义文件
├── internal/
│   ├── preset/          # 预设管理
│   ├── scanner/         # 图片扫描
│   ├── session/         # 会话管理
│   └── xmp/             # XMP 文件处理
└── data.local/          # 图片目录（默认）
```
