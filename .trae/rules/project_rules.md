# 开发指南

## 项目概览

**ImageFunnel** 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

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
- **frontend:** 修改前端代码后，使用 `pnpm check` 检查
- **powershell:** 脚本用当前 shell 直接运行 (直接 "./scripts/xxx.ps1")，不要额外调用 `pwsh` 或 `powershell.exe`
- **go:** 修改代码后，运行包测试并使用 `scripts/build.ps1` 构建
- **go:** 用 errors 包处理错误，避免直接比较
- **go:** 不要给查询方法添加 Get 前缀，直接用大写名称。比如不要 `GetSession()`，而应该直接 `Session()`
- **js:** 避免返回 null，直接使用 undefined 当作 null，但是参数支持 null
- **ts:** 直接使用 @/graphql/generated 生成的 GraphQL 类型，避免手动定义
- **vue:**　用声明式的方式代替命令式的维护（例如，用 computed 代替 watch 来维护状态）
- **vue:** 使用 defineModel 来定义双向绑定的模型
- **vue:** define使用的类型，直接定义在 defineXXX<{...}> 中，不要声明 Props 或 Emits 接口
- **graphql:** 用 fragments 来避免重复查询，命名不带后缀 Fragment（可以和类型名相同，生成的类型会自带 Fragment 后缀）
