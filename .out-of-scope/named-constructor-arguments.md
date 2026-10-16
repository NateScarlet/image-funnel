
# 具名参数（Options）构造领域实体

领域实体构造函数（如 `hook.FromRepository`）保持位置参数形式，不改为具名参数（options struct）。

## Why this is out of scope

具名参数的代价是**缺参数会被静默忽略**：调用方漏设某个字段时，Go 以零值填充而不报错。
位置参数传错位置时，只要相邻字段类型不同就会编译失败；只有当新增字段类型恰好与相邻字段一致时
才会被掩盖。也就是说位置参数在类型不一致时能提供编译期保护，而具名参数把这份保护整体移除，
错误更难在编译期发现。

## Prior requests

- #130 — "hook.FromRepository 的 9 个位置参数被 4 处用作 ID 编码器；decodeID 是死代码"
