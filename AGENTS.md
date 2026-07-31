# AGENTS.md

了解项目背景和上下文请查看 `CONTEXT.md`（以及 `CONTEXT-MAP.md`）；用户澄清后**必须**更新对应的 `CONTEXT.md` 以持久化上下文。

- **立即实现：**　立即实现所有用户要求的功能，不能偷懒用注释标记为以后实现
- **完整重构：**　内部接口更改应修改所有调用者使用新的接口，不得以向下兼容为由保留旧的接口，
- **注释**: 使用中文添加对理解上下文有帮助的注释，避免简单翻译代码
- **Region 注释**: 使用 `// #region {分组名称}` / `// #endregion` 包裹长段关联代码
- **不修改生成的代码**: 使用对应脚本重新生成
- **构建**: 优先使用 `scripts/build.ps1`，避免直接运行底层命令
- **临时产物**: 临时产物放在 `.scratch`，不主动清理

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
