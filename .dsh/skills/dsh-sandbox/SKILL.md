---
name: dsh-sandbox
description: 诊断与绕过 DSH 沙箱限制。命令报 spawn EPERM / Access is denied、Python 临时目录 PermissionError、pnpm install 长时间无输出死锁、registry 下载超时，或需要为沙箱做环境变量重定向、兼容补丁、申请完全权限（danger-full-access）时使用。
---

# DSH 沙箱问题的处理

DSH 沙箱施加两类约束，其余异常都是它们的组合：

1. **文件写入只放行 session workspace 与部分平台临时区**；动态新建的目录还受权限位校验。
2. **禁止子进程 stdio 管道**（Windows 命名管道）：`spawn` 默认即 pipe，故 Node 工具链大面积受击；`inherit` / `ignore` 不受限。

遇到失败先查下方速查表，命中则直接套已验证解法；未命中再走对照实验诊断。

## 已知陷阱速查

| 症状 | 根因 | 解法 |
|---|---|---|
| `spawn EPERM`（errno -4048），来自 esbuild / tsx / vite config 加载 | 工具经 JS API 以 stdio 管道 spawn 子进程 | 见「stdio 管道受限的替代方案」 |
| workspace 外路径报 `Access is denied`（go-build 缓存、全局 store 等） | 文件沙箱拦截 workspace 外写入 | 对应缓存/状态目录重定向进 `.scratch\`（如 `GOCACHE=.scratch\go-build`） |
| Python `PermissionError: [WinError 5]` 于 mkdtemp 目录内，sqlite 报 `unable to open database file` | `tempfile.mkdtemp` 固定以 0o700 建目录，沙箱内该权限的新建子目录内部不可访问；0o777 正常 | `TMPDIR` 指向 `.scratch\tmp`，并以 PYTHONPATH 注入 sitecustomize 将 mkdtemp 放宽为默认权限（参考 `scripts/python-sitecustomize/`） |
| pnpm install 卡在 resolve/reconcile：进程活着、零 TCP 连接、fetch-timeout 不触发、无重试日志 | 文件过滤层导致的无声死锁（silent deadlock），11.1.1 与 11.22.0 均复现；沙箱外同样命令十几秒完成 | **立即提权** danger-full-access 重跑同一命令，不做第三次绕行尝试 |
| registry 请求成批 `error (23)` 超时重试 | HTTPS_PROXY 指向半死代理，或镜像对新增包同步滞后 | 先清 `HTTP_PROXY`/`HTTPS_PROXY` 直连对照；仍慢则按 scope 在 `.npmrc` 把该 scope 指向官方源 |

## stdio 管道受限的替代方案

- **能换原生的换原生**：tsx/esbuild 类转译器换成运行时自带能力（Node ≥23.6 默认剥离 TS 类型，`node script.ts` 直跑）；rolldown/oxc/lightningcss 等 napi 原生绑定在进程内运行，天然免疫。
- **vite 8 专属**：首次路径解析会无条件 `exec("net use")` 收集映射盘信息，沙箱内必炸。补丁拦截该调用返回空输出即可——空输出使 vite 回退 `fs.realpathSync.native`，无映射盘环境下行为等价。参考 `frontend/vite.config.mts` 的内置兼容层（经 `createRequire` 替换内置模块对象，配合 `--configLoader native` 使补丁先于触发点生效）。
- **vitest**：`pool: "threads"`（worker_threads 进程内通信）替代默认的 forks；config 用 `--configLoader native` 加载，避开打包阶段的插件执行。

## 对照实验诊断新症状

速查表未命中时，用对照实验隔离根因——每次只改变一个条件，在两个上下文各跑一遍最小复现：

- **权限位假设**：同一目录操作分别在 `os.mkdir(d, 0o700)` 与 `0o777` 下执行（本次会话由此定位 Python 临时目录根因）。
- **网络假设**：用 `node -e "fetch(url)"` 直连测同一 URL 作为对照组。pwsh 的 `Invoke-WebRequest` 受系统代理与 TLS 栈影响，其失败不能证明网络不可达；Node fetch 与 pnpm 自身请求的速度差则是代理变量的证据（pnpm 读取 `HTTPS_PROXY`，Node fetch 不读）。
- **卡死类型判定**：`Get-NetTCPConnection -OwningProcess <pid>` ——有 ESTABLISHED 连接是网络等待，调大 timeout/重试有效；零连接且进程存活是无声死锁，重试参数无效，直接提权。
- **安装逐包定位**：`--reporter=ndjson` 输出逐包 fetch 事件，diff `started` 与 `completed` 得到精确卡点包列表。

## 红线

- **清理残留进程前先验明正身**：DSH 宿主本身就是 node 进程，误杀等于中断自己的工具调用。判定孤儿的依据是启动时间晚于本轮任务且父进程已退出；默认交给 `job_kill` 由运行时收尾，而不是手动杀进程树。
- **同一时刻只跑一个安装任务**：并行安装曾把系统内存耗尽，也让日志互相污染无法定位。
- **确认硬限制后立即提权**：同一操作第二次失败于同类 EPERM 时，申请完全权限重试是正确动作；继续寻找第三种绕行方式只会消耗轮次和用户耐心。

## 提权

`sandbox_permissions: danger-full-access` 只能作为刚被沙箱拒绝的操作的一次性重试，须附一句话 justification 说明该操作的哪一步被哪类约束阻塞。典型正当场景：pnpm reconcile 死锁、prebuild 依赖 tsx/esbuild 管道通信。探索性命令不提权。

## 本仓库已落地的适配

新会话遇同类问题时优先复用既有设施，避免重复发明：

- `scripts/test.ps1` — Go/Python/前端三件套统一入口（GOCACHE 重定向）
- `scripts/check-python.ps1` + `scripts/python-sitecustomize/` — Python 临时目录与 mkdtemp 权限适配
- `frontend/vite.config.mts` — net use 兼容层内联 + threads pool；前端 scripts 已带 `--configLoader native`
- `.npmrc` — `verify-store-integrity=false`（规避 store 校验死锁）与 scope registry
