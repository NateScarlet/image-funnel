# 研究结论：用筛选标注训练偏好小模型辅助初筛（含 retention 幸存者偏置重构）

> 研究范围：按 `spec.md` 的 7 个子问题组织，并纳入用户后续确认的 `example_hooks/retention.py` 自动清理事实。
> 一手来源优先：arXiv/会议论文、项目官方文档与源码、本项目源码。每条结论附链接。
> 说明：本文只做调研与建议，不修改项目源码。

---

## 0. 前提重构：retention 清理造成的「幸存者偏置」（本次重点）

### 0.1 已验证事实链（来自本项目源码与用户确认）

- `example_hooks/retention.py` 用 GraphQL 按 `rating` 过滤图片，按 `modTime` **升序**排序（`retention.py:60-84, 116-118`），把超出 `HOOK_MAX_RETAIN` 的最旧图片交给 `trashImages` 移到回收站（`retention.py:137-154`）。
- 跳过带非空 `note.content` 的图片（`retention.py:120-121`）。
- `movedCount` 明确包含「图片本体 + 同名配套文件（如 `.xmp`）」（`retention.py:153-159`），即被清理图片的 XMP 标注随图片一起离开活跃目录。
- 用户原话：「老的低星图片现在基本都没留」。

**推论：活跃数据集是被系统性负例删失（negative-censored）的。** 低星 / REJECT 样本被按时间批量移出，剩下的是「近期 + 高星」的正向偏置集合。这改变了整个研究问题的性质：问题不再是「标注够不够」，而是「判别边界所需的负例是否还存在」。

### 0.2 只用幸存正样本训练会发生什么

| 风险 | 机制 | 一手来源 |
|---|---|---|
| **学不到 REJECT 边界，退化为「全部保留」** | 只有正类时，判别式分类器没有负类可学；PU 学习指出，仅凭正例 + 未标注数据必须依赖额外假设（类先验 α、标注机制）才能训练出正确分类器，否则解不唯一 | [Learning from positive and unlabeled data: a survey, arXiv:1811.04820](https://arxiv.org/abs/1811.04820) |
| **把未标注当负例会引入混淆梯度** | 目标检测中把未标注区域当背景（PN）会产生「confusing learning signal」；改用 PU 损失后在 VOC/COCO 上稳定优于 PN | [Object Detection as a Positive-Unlabeled Problem, arXiv:2002.04672](https://arxiv.org/abs/2002.04672) |
| **PU 方法本身也会失效：SCAR 假设被破坏** | PU 学习的主流假设 SCAR 定义为「带标样本从正类分布中**完全随机**选出、与属性无关」。本项目的正例是按 rating 阈值 + 时间（modTime）选出的，明显不是完全随机 → 标准 PU 估计有偏 | [PU survey, Def.1 / §3.1.1, arXiv:1811.04820](https://arxiv.org/abs/1811.04820) |
| **无法评估 reject 的 precision** | 在 SCAR 下「recall 可估、precision 不可估」；没有保留负例，就无法知道模型建议 REJECT 的准确率 | [PU survey, §3.1, arXiv:1811.04820](https://arxiv.org/abs/1811.04820) |
| **选择偏置导致分布偏移/模型坍缩** | 有偏样本需重加权才能逼近真实分布；标签边缘 p(y) 改变而 p(x\|y) 不变即 label shift；低资源下有偏选择会优先保留局部流形样本、剪掉尾部模式，从而**加速模型坍缩** | [Sample Selection Bias Correction Theory, arXiv:0805.2775](https://arxiv.org/abs/0805.2775)；[BBSE label shift, arXiv:1802.03916](https://arxiv.org/abs/1802.03916)；[When Sample Selection Bias Precipitates Model Collapse, arXiv:2606.13732](https://arxiv.org/abs/2606.13732) |

### 0.3 是否必须用被清理的负例？

**结论：必须至少有负例的代表性样本，否则这条路走不通。**

- 判别边界由正负两侧共同决定；被 retention 清掉的正是「低星 + 旧」的负例，也是定义「什么该被排除」的关键样本。模型坍缩文献指出，有偏选择会剪掉分布尾部（[arXiv:2606.13732](https://arxiv.org/abs/2606.13732)），而偏好模型里「尾部」恰恰是奇怪的、难判的 reject 案例。
- 理论上可用 PU 学习在「正例 + 未标注池」上训练，但本项目两个前提都不满足：（1）SCAR 被 rating+时间选择破坏（[arXiv:1811.04820](https://arxiv.org/abs/1811.04820)）；（2）类先验 α 未知且随 retention 持续漂移（同上，PU 多数方法需要 α 作为输入）。PU 在有限正例下还需非负风险估计器来防止过拟合（[arXiv:1703.00593](https://arxiv.org/abs/1703.00593)），复杂度远高于直接保留负例代理。
- 回收站不是解决方案：`trashImages` 只是移动（`example_hooks/retention.py:137-154`），且回收站本身可能被再次清理；把训练数据建立在回收站上非常脆弱。
- **最低成本可行解：在 trash 发生前，把每条即将离开的标注「蒸馏」成轻量代理样本（标签 + 缩略图 + 哈希）持久化。** 见第 5、6 节。

---

## 1. 学习范式

**（1）三态 + 评分：优先当作「序数（ordinal）」而非名义三分类。**
KEEP / SHELVE / REJECT 有明确顺序，rating 也是有序分数。深度学习中把序数标签当普通类别交叉熵会丢失类间顺序信息，已有专门的序数回归方法（[Universally Rank Consistent Ordinal Regression, arXiv:2110.07470](https://arxiv.org/abs/2110.07470)；[Soft Labels for Ordinal Regression, CVPR 2019](https://doi.org/10.1109/cvpr.2019.00487)）。落地建议：先做 `KEEP vs REJECT` 二分类（最贴合产品决策），rating 作为样本置信度权重；三态作为可选的序数扩展。

**（2）SHELVE 的歧义：不要硬塞进某一类。**
SHELVE 语义是「暂缓/不确定」，本质是软标签或「未标注」。可两种处理并用：训练主分类器时把 SHELVE 降权或作为未标注样本（与第 0 节 PU 视角一致，[arXiv:1811.04820](https://arxiv.org/abs/1811.04820)）；需要三态输出时再作为序数中间档。把它当硬负例会污染边界，当硬正例会引入噪声。

**（3）相对批次偏好 vs 绝对偏好。**
- 「在 N 张里挑好的」是 pairwise/listwise 排序问题：Bradley–Terry 配对比较（[Revisiting the Bradley-Terry model, DOI:10.7465/jkdi.2013.24.5.1089](https://doi.org/10.7465/jkdi.2013.24.5.1089)；[Inductive Pairwise Ranking, arXiv:1702.02661](https://arxiv.org/abs/1702.02661)）与 BPR 隐式反馈排序（[arXiv:1205.2618](https://arxiv.org/abs/1205.2618)）是标准范式；人类偏好学习也普遍用成对比较（[Christiano et al. 2017, arXiv:1706.03741](https://arxiv.org/abs/1706.03741)，仅需不到 1% 交互反馈）。优势：对全局审美尺度漂移不敏感；劣势：必须保存「哪些图在同批次被比较」的上下文。
- 绝对偏好（KEEP/REJECT）直接对应 XMP 里的 `action`，无需批次上下文，工程上最省。
- **本研究建议**：主任务用绝对三态/二分类；可选地从「同一会话/批次内 KEEP 必优于 REJECT」自动构造配对约束，但需要在轻量样本里新增 `batch/session id` 字段（当前 XMP 不含此信息，见第 6 节）。注意：显式标注已存在，BPR 式隐式反馈不是主要范式；只有第 0 节的删失场景才把「未标注」引入视野。

---

## 2. 模型选择（几十～几千标注，本地/CPU 可行）

| 方案 | 数据需求 | 算力 / 本地性 | 一手来源与评价 |
|---|---|---|---|
| **CLIP/OpenCLIP + linear probe（首选）** | CLIP 论文：zero-shot 约等于同特征空间上的 **4-shot** logistic regression，且大致追平当时最好的 **16-shot** 分类器；ImageNet 上 16-shot logistic regression 追平 zero-shot。三分类粗算 ~12–48 张即可起步，个人审美建议数百张 | 一次前向提特征 + scikit-learn `LogisticRegression`，**CPU 可跑**；官方 README 直接给出线性探针示例与 `cpuonly` 安装说明 | [CLIP, arXiv:2103.00020](https://arxiv.org/abs/2103.00020)；[OpenAI CLIP README](https://github.com/openai/CLIP/blob/main/README.md) |
| **Tip-Adapter（训练-free 缓存）** | 少量样本构造 key-value cache，无需反向传播；再微调极少数 epoch 即可收敛（比已有方法少 10× epoch） | 仍需 CLIP 前向；无需训练基础设施，最省事 | [Tip-Adapter, arXiv:2111.03930](https://arxiv.org/abs/2111.03930)（另有 [2207.09519](https://arxiv.org/abs/2207.09519)） |
| **MobileNetV3 / EfficientNet 微调** | 需预训练权重 + 较强增强；几十～几百张容易过拟合，冻结 backbone 只训分类头更稳 | MobileNetV3 明确为**手机 CPU** 调优；EfficientNet-B7 比当时最优 ConvNet 小 8.4×、推理快 6.1× | [MobileNetV3, arXiv:1905.02244](https://arxiv.org/abs/1905.02244)；[EfficientNet, arXiv:1905.11946](https://arxiv.org/abs/1905.11946) |
| **LoRA / 参数高效微调** | 面向「标注上千 + 领域明显偏移」阶段 | LoRA 可把可训练参数减少 10,000×、GPU 显存减少 3×（以 GPT-3 175B 为例），但本项目当前规模用不上 | [LoRA, arXiv:2106.09685](https://arxiv.org/abs/2106.09685) |
| **VLM 文本提示（冷启动）** | 零标注，用自然语言描述偏好 | 只用 CLIP 文本/图像编码，CPU 可跑 | CLIP 零样本可用，但论文测得零样本普遍比 linear probe 低 **10–25 个点**（[arXiv:2103.00020](https://arxiv.org/abs/2103.00020)）；个性化审美高度主观（[arXiv:2106.07488](https://arxiv.org/abs/2106.07488)），只能作冷启动/兜底 |

**结论**：技术路线应为「零样本提示冷启动 → CLIP 特征 + 线性探针/Tip-Adapter → （标注上千后）轻量 adapter/LoRA」。CLIP 特征 512 维、float32 约 **2 KB/张**（[HuggingFace `clip-vit-base-patch32` config.json: `projection_dim: 512`](https://huggingface.co/openai/clip-vit-base-patch32/blob/main/config.json)），本身已是极省存储的训练素材。

---

## 3. 数据量与主动学习

- **可用量级**：CLIP 论文给出 0/1/2/4/8/16/…/128-shot 的系统评估，4-shot 与 zero-shot 相当、16-shot 追平强监督特征基线（[arXiv:2103.00020](https://arxiv.org/abs/2103.00020)）。对三分类，几十张标注就能得到「比随机好」的初筛；数百到数千张才谈得上稳定。但前提是**类别平衡且包含负例**——第 0 节的幸存者偏置使「名义上的张数」失去意义。
- **主动学习确能省标注**：核心集（core-set）选择可显著优于随机采样（[arXiv:1708.00489](https://arxiv.org/abs/1708.00489)）；难例挖掘（OHEM）表明数据集中「大量简单样本 + 少量难样本」，自动挑难例能提升效率与效果（[arXiv:1604.03540](https://arxiv.org/abs/1604.03540)）。
- **但要泼冷水**：一项覆盖 **460 万**超参组合的大规模研究发现，AL 效果对实现细节与超参高度敏感，且业界对 AL 有效性「缺乏信任」是常态（[Survey of Active Learning Hyperparameters, arXiv:2506.03817](https://arxiv.org/abs/2506.03817)）。不要把 AL 当成必然收益。
- **对本项目最关键的一点**：AL 只能从「未标注池」里挑样本；若池子已被 retention 删掉负例，AL 只会不断挑出同质性正例，**无法修复第 0 节的问题**。因此顺序是「先留存负例代理 → 再谈主动学习」。

---

## 4. 标注质量、偏好漂移

- **噪声**：DNN 对标签噪声敏感，且**有偏噪声比无偏噪声危害更大**（[Deep Learning with Label Noise, arXiv:2205.14299](https://arxiv.org/abs/2205.14299)；[Label noise in segmentation networks, arXiv:2107.02189](https://arxiv.org/abs/2107.02189)）。审美标注本身高度主观（[BDN / AVA, arXiv:1601.04155](https://arxiv.org/abs/1601.04155)；[艺术图像审美主观性, arXiv:2303.15166](https://arxiv.org/abs/2303.15166)）；小型多模态模型甚至表现出更强刻板偏置（[AesBiasBench, arXiv:2509.11620](https://arxiv.org/abs/2509.11620)）。
- **缓解**：用 rating 作置信度（高星/剧烈反对 = 高置信，SHELVE = 低置信），软标签/序数损失，时间衰减加权，并优先用「同批次相对比较」降低全局尺度漂移影响。
- **漂移真实存在且可建模**：时序矩阵分解追踪个体偏好漂移，在合成数据/Ciao 上 RMSE 改善约 20% / 5%（[Temporal MF, arXiv:1510.05263](https://arxiv.org/abs/1510.05263)）；流式推荐用「时间感知采样 + 新旧数据分层」兼顾概念漂移与长期偏好（[arXiv:2009.06824](https://arxiv.org/abs/2009.06824)）。XMP 已含 `timestamp`，可直接用于时间衰减——这是本项目现成优势。

---

## 5. 存储策略（关键）：是否需要原图？轻量代理能省多少？

### 5.1 训练不需要原图——一手依据

- OpenAI CLIP 的预处理源码是 `Resize(n_px, BICUBIC)` + `CenterCrop(n_px)`，`n_px = model.visual.input_resolution`（ViT-B/32 为 224）；即**模型一辈子只看到 224×224**（[clip/clip.py](https://github.com/openai/CLIP/blob/main/clip/clip.py)、[README](https://github.com/openai/CLIP/blob/main/README.md)）。
- OpenCLIP 官方模型表显示主流模型分辨率为 **224px / 256px**（少量 336/384/448）（[OpenCLIP README](https://github.com/mlfoundations/open_clip/blob/main/README.md)）。
- 因此：**保存一张 224×224 缩略图，就保留了 CLIP 类模型会消费的全部像素信息**；日后要换 backbone 也可随时从缩略图重算特征。

### 5.2 量化节省（像素 + 文件 + 本地实测）

| 维度 | 原始样本（用户实测：1024–2304px PNG，数 MB） | 轻量代理 | 节省 |
|---|---|---|---|
| 像素数 | 2304² = 5,308,416；1024² = 1,048,576 | 224² = 50,176 | **约 21×（对 1024px）/ 106×（对 2304px）** |
| 文件体积（本项目本地实测，**合成样本，非用户真实图片**） | 合成 2048² plasma PNG = **21.64 MB** | 224² WebP q75 = **4,036 B**；AVIF q50 = **3,618 B** | **约 5,000×**（该 PNG 为不可压缩噪声，属最坏情况） |
| 文件体积（本地实测，项目内简单图标 JPG） | 1920² JPG = 109,816 B | 224² WebP q75 = 1,872 B；AVIF q50 = 1,850 B | 约 **59×** |
| 通用压缩依据 | PNG/JPEG | WebP lossless 比 PNG 小 **26%**；lossy 比同质量 JPEG 小 **25–34%**；lossy WebP 通常比 PNG 小 **3×** | [Google WebP 官方文档](https://developers.google.com/speed/webp) |
| 特征向量（可选替代/补充） | — | 512 维 float32 = **2 KB/张** | [HF config.json](https://huggingface.co/openai/clip-vit-base-patch32/blob/main/config.json) |

> 本地实测文件位于 `.scratch/2026-11-18-preference-model-research/`（`synth2048.png`、`synth224.webp`、`synth224.avif`、`thumb224.webp`、`thumb224.avif`），可用 `magick identify` 复现。真实 AI 图片的压缩比预期落在上述两端之间，**保守取 200–1000×**。

**建议的轻量样本三件套（总 ~2–20 KB/张，相比数 MB 原图约 3 个数量级）：**

- **(a) 标签本身**：`action`(KEEP/SHELVE/REJECT)、`rating`、`timestamp`、图片标识、**新增 `batch/session id`**（支撑第 1 节相对偏好）。XMP 已提供前三者（本项目 spec 已验证事实）。
- **(b) 低分辨率缩略图**：224×224 WebP/AVIF（几 KB 级）。本项目已有 256w 变体管道与 ffmpeg/SVT-AV1 编码器（`internal/infrastructure/ffmpeg/encoder.go` 实测 256/512/1024/2048 档位），但**变体缓存会过期**，必须写到独立持久目录，不能依赖变体缓存。
- **(c) 稳定标识**：内容哈希（文件字节 SHA-256）+ 感知哈希（pHash）。感知哈希对缩放/裁切/轻微改动鲁棒、用于近重复检索与去重（[State of the Art: Image Hashing, arXiv:2108.11794](https://arxiv.org/abs/2108.11794)）。两者都要存：内容哈希精确、感知哈希在重编码后仍可关联。

---

## 6. 落地路径：针对本项目的分阶段建议

### 阶段 0（现在，几乎零成本；最高优先级）——堵住负例流失

- **在 `trashImages` 之前，retention hook 先为每张待清理图片导出一条轻量训练样本**。源码里 `excess` 列表（`retention.py:118-128`）正好是即将被 trash 的图片集合，插入点明确；导出失败应**阻止 trash**（快速失败，避免再次静默丢失负例）。
- **导出内容**：第 5 节三件套，落到**活跃目录之外的、retention 不会扫描的持久目录**（例如项目内 `data/preference-archive/`，按图片哈希分文件 + 一个 append-only `samples.jsonl`）。不要用 `.scratch`（临时产物目录），也不要写进变体缓存（会过期）。
- **与 ADR-0001 的张力及最小化处理**：`docs/adr/0001-xmp-sidecar-as-source-of-truth.md`（XMP 为唯一持久化来源）反对第二份「真相源」。处理办法：把归档定义为**只增不改的派生快照（derived archive）**，不是新的 source of truth——它在图片被 trash 的瞬间从 XMP 复制字段生成，不参与活跃图片的读写；活跃图片的真相仍是 XMP。这样既保留负例信息，又不违背 ADR-0001 的核心约束。
- **不引入数据库**：用 JSONL + 缩略图文件即可，无需 SQLite/Postgres（ADR-0001 已论证过单机场景的过度设计）。
- **注意 skip-note 造成的二次偏置**：带 note 的图片被跳过不清理，意味着「有笔记的负例」会异常存活；导出样本时不应因 note 而跳过（note 只影响是否清理，不影响标注有效性）。
- 更理想（后续）是把导出时机前移到 Mark/Commit，但对当前「立刻止血」而言，retention 时导出是改动最小、收益最大的切入点。

### 阶段 1（有足够平衡样本后）——训练与评估

- 用归档样本训练 CLIP 线性探针 / Tip-Adapter（第 2 节）；**测试集必须包含被保存的负例**，否则 REJECT 的 precision 无法测量（第 0 节，PU survey）。
- 评估指标：`precision@k`（建议 REJECT 的前 k 张中用户确认比例）、REJECT 召回率、**人工接管/推翻率**。scikit-learn 的 `top_k_accuracy_score` 给出 top-k 判定的标准定义（[scikit-learn docs](https://scikit-learn.org/stable/modules/generated/sklearn.metrics.top_k_accuracy_score.html)），precision@k 需按此自定义。
- 建议门槛：例如 `precision@10 > 70%` 且用户推翻率显著下降，才把模型输出作为「建议」接入 UI；初期**不自动执行 REJECT**，只做初筛排序/高亮。

### 阶段 2（标注上千且漂移明显）——轻量增强

- 引入主动学习挑不确定样本（第 3 节，注意超参敏感性），时间衰减/定期重训（第 4 节），必要时上 LoRA/小 adapter。始终保留人工最终决定权。

---

## 7. 风险与反例（什么情况下走不通）

1. **负例没被保住**（当前最大风险）：只要 retention 继续静默清掉低星图片而不导出代理，任何训练都会坍缩成「全部保留」或学到有偏边界（[arXiv:1811.04820](https://arxiv.org/abs/1811.04820)、[arXiv:2606.13732](https://arxiv.org/abs/2606.13732)）。**这是先决条件，不是优化项。**
2. **标注量太小**：几十张以下或每类 <10 张时，线性探针不稳定；零样本提示可能反而更稳（[arXiv:2103.00020](https://arxiv.org/abs/2103.00020)）。
3. **偏好太主观/自相矛盾**：审美评估高度主观（[arXiv:1601.04155](https://arxiv.org/abs/1601.04155)、[arXiv:2303.15166](https://arxiv.org/abs/2303.15166)），小模型偏置更强（[arXiv:2509.11620](https://arxiv.org/abs/2509.11620)）；若同一用户跨时间一致性差，模型学的是噪声。
4. **批次相对偏好无法泛化**：配对/排序依赖批次上下文；批次构成变化会导致排序漂移，且当前 XMP 不记录 batch id（第 1、6 节）。
5. **维护成本大于收益**：AL 超参敏感、漂移需重训、缩略图/哈希管道要 QA；若实际使用频率低、标注长期停在几十张，不应训练模型——只做第 0 节的标注留存即可。
6. **把回收站当数据仓**：trash 非永久且可能被二次清理（`example_hooks/retention.py`），不能作为训练数据依赖。

---

## 来源清单

### 论文（arXiv / 会议）
- Christiano et al., *Deep reinforcement learning from human preferences* — https://arxiv.org/abs/1706.03741
- Rendle et al., *BPR: Bayesian Personalized Ranking from Implicit Feedback* — https://arxiv.org/abs/1205.2618
- *Revisiting the Bradley-Terry model and its application to information retrieval* — https://doi.org/10.7465/jkdi.2013.24.5.1089
- *Inductive Pairwise Ranking: Going Beyond the n log(n) Barrier* — https://arxiv.org/abs/1702.02661
- Radford et al., *Learning Transferable Visual Models From Natural Language Supervision (CLIP)* — https://arxiv.org/abs/2103.00020
- Zhang et al., *Tip-Adapter: Training-free CLIP-Adapter for Better Vision-Language Modeling* — https://arxiv.org/abs/2111.03930 ；https://arxiv.org/abs/2207.09519
- Hu et al., *LoRA: Low-Rank Adaptation of Large Language Models* — https://arxiv.org/abs/2106.09685
- Tan & Le, *EfficientNet* — https://arxiv.org/abs/1905.11946
- Howard et al., *Searching for MobileNetV3* — https://arxiv.org/abs/1905.02244
- Sener & Savarese, *Active Learning for Convolutional Neural Networks: A Core-Set Approach* — https://arxiv.org/abs/1708.00489
- Shrivastava et al., *Training Region-based Object Detectors with Online Hard Example Mining* — https://arxiv.org/abs/1604.03540
- *Survey of Active Learning Hyperparameters: Insights from a Large-Scale Experimental Grid* — https://arxiv.org/abs/2506.03817
- *A Survey of Active Learning for Natural Language Processing* — https://arxiv.org/abs/2210.10109
- *Universally Rank Consistent Ordinal Regression in Neural Networks* — https://arxiv.org/abs/2110.07470
- *Soft Labels for Ordinal Regression*, CVPR 2019 — https://doi.org/10.1109/cvpr.2019.00487
- Bekker & Davis, *Learning from positive and unlabeled data: a survey* — https://arxiv.org/abs/1811.04820
- Kiryo et al., *Positive-Unlabeled Learning with Non-Negative Risk Estimator* — https://arxiv.org/abs/1703.00593
- *Learning from positive and unlabeled examples – Finite size sample bounds* — https://arxiv.org/abs/2507.07354
- *Object Detection as a Positive-Unlabeled Problem* — https://arxiv.org/abs/2002.04672
- *Sample Selection Bias Correction Theory* — https://arxiv.org/abs/0805.2775
- *Detecting and Correcting for Label Shift with Black Box Predictors* — https://arxiv.org/abs/1802.03916
- *When Sample Selection Bias Precipitates Model Collapse* — https://arxiv.org/abs/2606.13732
- *Temporal Matrix Factorization for Tracking Concept Drift in Individual User Preferences* — https://arxiv.org/abs/1510.05263
- *Stratified and Time-aware Sampling based Adaptive Ensemble Learning for Streaming Recommendations* — https://arxiv.org/abs/2009.06824
- *Deep Learning with Label Noise: A Hierarchical Approach* — https://arxiv.org/abs/2205.14299
- *Label noise in segmentation networks: mitigation must deal with bias* — https://arxiv.org/abs/2107.02189
- *State of the Art: Image Hashing* — https://arxiv.org/abs/2108.11794
- *User-Guided Personalized Image Aesthetic Assessment based on Deep Reinforcement Learning* — https://arxiv.org/abs/2106.07488
- *Brain-Inspired Deep Networks for Image Aesthetics Assessment* — https://arxiv.org/abs/1601.04155
- *Towards Artistic Image Aesthetics Assessment* — https://arxiv.org/abs/2303.15166
- *AesBiasBench: Evaluating Bias and Alignment ... Personalized Image Aesthetic Assessment* — https://arxiv.org/abs/2509.11620

### 官方文档 / 源码
- OpenAI CLIP README（CPU 支持、linear-probe 示例） — https://github.com/openai/CLIP/blob/main/README.md
- OpenAI CLIP `clip/clip.py`（Resize/CenterCrop 到 input_resolution） — https://github.com/openai/CLIP/blob/main/clip/clip.py
- OpenCLIP README（模型分辨率表、linear probe 脚本） — https://github.com/mlfoundations/open_clip/blob/main/README.md
- HuggingFace `openai/clip-vit-base-patch32` config.json（`projection_dim: 512`） — https://huggingface.co/openai/clip-vit-base-patch32/blob/main/config.json
- Google WebP 官方页（压缩率数据） — https://developers.google.com/speed/webp
- scikit-learn `top_k_accuracy_score` 文档 — https://scikit-learn.org/stable/modules/generated/sklearn.metrics.top_k_accuracy_score.html

### 本项目源码 / 文档（工作区内）
- `example_hooks/retention.py` — `C:\Workspaces\image-funnel\example_hooks\retention.py`
- `docs/adr/0001-xmp-sidecar-as-source-of-truth.md` — `C:\Workspaces\image-funnel\docs\adr\0001-xmp-sidecar-as-source-of-truth.md`
- `internal/infrastructure/ffmpeg/encoder.go`（256/512/1024/2048 档位实测） — `C:\Workspaces\image-funnel\internal\infrastructure\ffmpeg\encoder.go`
- 本地缩略图体积实测产物 — `C:\Workspaces\image-funnel\.scratch\2026-11-18-preference-model-research\`
