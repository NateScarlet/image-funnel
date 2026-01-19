---
trigger: always_on
---

# 项目概览

**ImageFunnel** 是一个专门用于 AI 生成图片筛选的 Web 应用，通过简单的工作流帮助用户从大量生成结果中快速筛选出优质图片。

**核心特点：**

- **无侵入式元数据管理**：使用 XMP Sidecar 文件存储筛选结果，不修改原始图片
- **移动优先的 Web 界面**：支持手势操作和键盘快捷键
- **三态分类工作流**：保留/稍后再看/排除，避免决策疲劳
- **专业工具兼容**：Adobe Lightroom/Bridge、XnView 等可直接读取评分

## 技术栈

- **后端**：Go + gqlgen（高性能并发处理，实时 GraphQL 接口）
- **前端**：Vue 3 + TypeScript + Tailwind（快速开发响应式移动界面）
- **元数据**：XMP Sidecar 文件（遵循 Adobe 标准）
- **存储**：文件系统（零额外数据库）


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