# ADR 0010: Accept 头同 q 值的等价性与转码目标固定优先级

## 状态
Accepted

## 背景

`/image` 的格式协商依赖客户端 Accept 头（见 [ADR 0003](./0003-accept-header-image-negotiation.md)）。实现中对同 q 值的多条 media-range 用 specificity 降序排序，通配符（`*/*`、`image/*`）因此排在具体类型之后，客户端书写顺序被覆盖。这引出一个必须明确回答的问题：Accept 头里同 q 值的书写顺序有没有语义？

RFC 9110 §12.5.1 给出的答案是**没有**。原文示例：

    Accept: text/plain; q=0.5, text/html, text/x-dvi; q=0.8, text/x-c

被解释为 "text/html and text/x-c are the equally preferred media types"——同 q 值即同等偏好，服务端在同等偏好中有裁量权。标准从未赋予同 q 值的书写顺序任何偏好语义。客户端要表达顺序偏好必须用 q 值。

与此同时，选转码目标（AVIF / WebP）在同 q 值时必须给出确定答案，否则同一请求的结果不可预测、无法被测试稳定断言。

## 决定

1. **同 q 值不按书写顺序。** q 值是内容协商中唯一的偏好来源。不引入「同 q 时按 Accept 书写顺序」这一超出 RFC 的私有约定。
2. **同 q 值时转码目标固定 AVIF > WebP。** 二者 q 值相同时优先 AVIF（压缩效率更好）；q 值不同时仍按 q 值选。

## 考虑过的选项

- **同 q 时按 Accept 书写顺序**：与 RFC 语义冲突，并会让实现依赖一个标准不承认的输入特征。拒绝。
- **同 q 时沿用现有排序的偶然顺序**：现行排序基于不稳定排序算法，不保证并列时保序，不能作为契约。拒绝。
- **向社区常见实现看齐**：gin 的 `Context.Negotiate` 实际丢弃 q 值（其 `parseAccept` 直接砍掉分号之后的全部内容），退化为书写顺序，比本实现更远离 RFC。不采用。

## 后果

- `Accept: */*, image/webp` 与 `Accept: image/webp, */*` 产出相同结果，这是**有意行为**，不是缺陷。未来读到协商排序逻辑的人不应把它当作 bug「修复」——要恢复顺序语义需要先推翻本 ADR。
- 浏览器典型头 `image/avif, image/webp, image/*, */*;q=0.8` 中 avif 与 webp 同 q，按决定 2 得到 AVIF。
- 转码目标的取舍比较的是各受支持格式的**有效 q**：通配符不产出转码目标元素，但会让 avif/webp 以通配符的 q 参与比较。因此 `Accept: */*` 下二者同为 q=1，按决定 2 得到 AVIF；`Accept: image/webp;q=0, */*;q=0.9` 下 webp 的有效 q 为 0，只剩 AVIF。
- 本 ADR 只覆盖排序与并列取舍。RFC 9110 §12.5.1 的 specificity 覆盖规则与 `q=0` 语义属于协议既有要求，不是本项目的权衡，不在此记录。
