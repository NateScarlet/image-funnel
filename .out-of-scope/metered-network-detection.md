
# 检测流量计费网络并提示

应用不提供“检测当前是否走移动数据并在进入时提示”的能力：不引入 Network Information API 探测、不做服务端 IP 网段启发式、也不提供让用户手动声明网络类型的开关。

## Why this is out of scope

核心目标是 Firefox 桌面与 Firefox Android，而这两个环境都不具备可用的网络类型信号：

- **Firefox 桌面从未向 Web 暴露 Network Information API**，`navigator.connection` 为 `undefined`。
- **Firefox Android 在 99 起移除了 `type`**（因指纹识别暴露面，Firefox bug 1637922），目标版本已在移除之后。

服务端 IP 启发式同样不成立：用户实际通过 Tailscale 访问，`100.64.0.0/10` 的地址在服务端看来是私有网段，底层却可能承载蜂窝流量，检测会稳定判定为局域网而永不提示。在 Tailscale 接管默认路由的情况下，即便 Chromium 的操作系统级连接类型也不可信。

手动开关被否决的理由：需求动机是“用户忘了开 WiFi”，而手动开关要求用户记得切换另一个设置，把记忆负担平移而非消除，没有解决任何问题。

结论是环境层面不存在可靠信号，功能无法实现，而非实现成本过高。

## Prior requests

- #152 — “检测流量计费网络并提示”，经 grilling 确认无可靠检测信号后关闭。
