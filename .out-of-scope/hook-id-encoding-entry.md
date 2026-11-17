# 对根暴露 Hook ID 编码入口

hook 域不对外（domain 包之外）提供「原始配置 ID → 领域 ID」的编码入口：`encodeID` / `decodeID` 保持包内非导出，外部层级不得直接调用，也不新增等价的导出函数作为替代通道。

## Why this is out of scope

ID 的编解码是领域内部实现细节，由 `CODING_STANDARDS.md` 的 Go 章节明确规定：

- **ID 编码不导出**：`encodeID` 和 `decodeID` 均为非导出函数，外部任何层级不得直接调用。需要将 ID 翻译为路径或获取领域对象时，应通过领域 Service 或 Repository 获取领域对象后从中读取属性。
- **仓库构造权**：领域实体的原始构造函数不应导出供包外调用，外部构造入口统一为 `FromRepository`，该方法内部调用非导出的构造函数与 `encodeID` 生成 ID。只有仓库实现有权限构造领域对象。
- **通过仓库获取 ID**：外部代码需要领域实体的 ID 时，应通过仓库获取领域对象后调用 `.ID()`，不得自行编码。

因此「导出 `encodeID`」或「新增一个等价的导出编码函数」都会把领域内部细节泄漏到基础设施层，破坏上述边界。配置层经 `FromRepository` 构造实体再读 `.ID()` 正是规范要求的取 ID 方式——位置参数中有若干恒定值，是这一边界的代价，而不是应当消除的缺陷。

对 #130 曾提过的「具名参数 / options 构造」方向另有否决记录，见 [named-constructor-arguments.md](named-constructor-arguments.md)。

## Prior requests

- #130 — "hook.FromRepository 的 9 个位置参数被 4 处用作 ID 编码器；decodeID 是死代码"（维护者于 2026-10-19 明确否决导出 `encodeID` 的方向）
