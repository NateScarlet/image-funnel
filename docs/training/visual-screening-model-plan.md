# 训练样本归档数据规范与视觉初筛模型计划（一期）

## 1. 背景与目标

图片筛选每次 Mark 产生 KEEP / SHELVE / REJECT 决策，Commit 时写入 XMP。既有 `retention.py` 会按评分与修改时间把旧低星图片连同 XMP 一起移入回收站，负例被系统性删失，活跃数据集变成「近期 + 高星」的正向偏置。只用幸存正例训练会坍缩为「全部保留」，PU 学习也因 SCAR 假设被 rating + 时间选择破坏而失效。

因此在一期把标注在流失前蒸馏为轻量、可长期留存的训练样本，并明确训练对象。**本文件只覆盖采集与计划，不训练模型本身。**

采集位置是主应用 `Service.Commit`，不是 `post_commit_session` 钩子：钩子只拿到目录相对路径、GraphQL `Image` 不暴露 `action`、rating 未变时 commit 直接跳过写盘、且钩子是 `go run(...)` fire-and-forget，失败只记日志，丢一次就永久缺一个负例。

## 2. 数据规范

### 2.1 采集时机与零值安全

- 每条实际产生的标记决策（含 rating 未变、仍被用户决策过的图片）产生一条事件。
- 文件不存在或 ID 不匹配（外部修改）的图片不产生事件。
- `Service.Commit` 接收必传的 `SampleRecorder`；未启用归档时注入空实现 `NullSampleRecorder`，提交行为与现状完全一致。
- 归档失败通过 Commit 返回的错误向调用者可见，不 fire-and-forget、不只记日志；XMP 作为唯一真相源的写入不被阻断。

### 2.2 事件字段

事件写入 `data/samples.jsonl`（append-only，UTF-8，每行一个 JSON 对象）：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `imageId` | string | 图片标识（领域 ID 字符串） |
| `directoryId` | string | 目录标识 |
| `sessionId` | string | 会话标识；指纹缺失时作为后备分组键 |
| `round` | int | 筛选轮次 |
| `action` | string | `KEEP` / `SHELVE` / `REJECT` |
| `rating` | int | 目标评分 |
| `timestamp` | string | RFC3339Nano 决策时间 |
| `thumbnailCid` | string | 缩略图 CID，通过它引用字节，不重复写 JSONL |
| `fingerprint` | string | 字符串指纹，可空；为空时不参与指纹分组 |

### 2.3 缩略图内容寻址布局

- 提交当下从原图生成固定 **224×224 居中裁剪 WebP** 缩略图（质量 75，数 KB 级），写入独立持久目录，不依赖会过期的变体缓存。
- CID 计算：SHA-256 摘要 → CIDv1（version 1 / codec raw 0x55 / sha2-256 multihash 0x12）→ base32 大写无填充，首字符为多基前缀 `B`。
- 令 `h = CID` 去掉首字符 `B`；分片目录取 `h` 的倒数第 3、2 位；文件路径为 `data/cas/<分片>/<h>.data`。
- 分片布局与用户其它 IPFS 兼容 CAS 项目（如 `obsidian-content-addressed-attachments` 的 `formatRelPath`）一致，便于跨项目共用工具。
- 相同字节只落盘一次；JSONL 只记 CID 引用。
- 根目录固定为项目内 `data/cas/`，不在活跃图片目录、不被 retention 扫描、不在变体缓存路径上；由环境变量 `IMAGE_FUNNEL_ENABLE_TRAINING_SAMPLE_ARCHIVE=true` 显式开启，默认关闭。

### 2.4 字符串指纹及其局限（必须随数据一起传播）

提取方式：递归遍历完整 ComfyUI `prompt` JSON，收集**所有字符串类型的值**，排序去重后以换行连接并计算 SHA-256；数字（种子、steps、cfg、尺寸）被自然忽略；不硬编码任何节点类型或字段名；无 PNG 元数据、解析失败或没有字符串时留空。

**已知局限（启发式，不得当作精确的「同提示词」语义）：**

- 收集到的字符串可能混入节点类型名、模型名、节点 ID、自定义节点写入的随机串或临时路径，导致同配置算出不同指纹；
- 也可能把不同配置的图归为一组；
- 不存提示词正文与完整 workflow JSON（实测元数据 50–58KB/张，会压垮 2–20KB 的轻量样本）。

`sessionId` **不是**同提示词分组键。`comfyui_remix.toml` 表明会话提交后 `/remix` 会对 4 星图应用 `--add` / `--remove` / `--lora` / `--cfg` / `--aspect` 等编辑，编辑项取笛卡尔积、种子按 job 变化；同一批筛选的图配置可以完全不同。`sessionId` 只作为指纹缺失时的后备弱分组键，表示「同批筛选」。

### 2.5 读取与切分

- `ReadRecords(dataRoot)` 流式读取 JSONL；`LatestPerImage(records)` 按图片标识只保留 timestamp 最大的一条（「留到最后」），timestamp 相同时以轮次较大者为准。保留旧记录供后续时间衰减使用。
- `Split(records, testRatio)` 按分组键整组贪心分配，**禁止随机切分**：优先 `fingerprint`（可用时），否则回退 `sessionId`。报告必须注明上述分组键局限。

## 3. 目标训练对象计划（一期）

- **任务**：KEEP vs REJECT 二分类；SHELVE 降权或视为未标注，不硬塞类别。
- **特征**：CLIP / OpenCLIP ViT-B/32 图像特征（224×224 输入，512 维），可由归档的 224² 缩略图随时重算。
- **模型**：线性探针（sklearn `LogisticRegression`，CPU 可跑）起步；备选 Tip-Adapter。
- **数据量级**：CLIP 论文 4-shot≈zero-shot、16-shot≈强基线；数十张起步即可「比随机好」，稳定建议数百到数千，且必须类别平衡（含负例）。
- **数据切分**：按第 2.5 节分组切分；单独报告「组内 KEEP vs REJECT」区分度；报告中注明分组键局限。
- **评估**：测试集必须含被保存的负例；指标 precision@k、REJECT 召回、人工推翻率。建议门槛 precision@10 > 70% 才接入 UI。
- **使用方式**：初期只输出建议 / 排序 / 高亮，**不自动执行 REJECT**，人工保留最终决定权。
- **不做**：LoRA / 主动学习 / 提示词条件模型（留到后续阶段，视样本量与漂移情况再定）；不训练模型本身（本 issue 仅交付采集 + 计划）。

## 4. 配置与运维

- 环境变量 `IMAGE_FUNNEL_ENABLE_TRAINING_SAMPLE_ARCHIVE=true` 开启；未设置、无效值或 `false` 均关闭（零值安全）。
- 开启后启动时创建 `data/cas/` 与 `data/samples.jsonl`；日志会打印解析后的数据目录绝对路径。
- 失败可见：缩略图生成、CAS 写入、JSONL 追加任一失败都会使 `Service.Commit` 返回错误。
