# Changelog

## [1.5.0](https://github.com/NateScarlet/image-funnel/compare/v1.4.1...v1.5.0) (2026-06-05)


### Features

* add app favicons for branding and visual identity ([250faef](https://github.com/NateScarlet/image-funnel/commit/250faef2056b5c7de84a55abd5884a9aaf610b1d))
* allow opening navigation links in a new tab ([333bc2f](https://github.com/NateScarlet/image-funnel/commit/333bc2f3483527c78863fbf255dfecd8977b033e))
* always show memo icon in viewport and support calling out in fullscreen ([9d36c06](https://github.com/NateScarlet/image-funnel/commit/9d36c069f615896ab736d2e94ba89e138deae342))
* avoid mark image before loaded ([c760b26](https://github.com/NateScarlet/image-funnel/commit/c760b26087b0735739059bcc73b758253add20a3))
* **frontend:** 在目录浏览视图中支持一键返回最近的会话 ([76f3947](https://github.com/NateScarlet/image-funnel/commit/76f3947801b09597b6e623c6af75463bc6fcb670))
* **frontend:** 支持快捷键设置/清除图片颜色标签并重构会话快捷键 ([32a2a08](https://github.com/NateScarlet/image-funnel/commit/32a2a0818391878e25ccef866d09b84f32f226bb))
* **frontend:** 添加Memo上滑动画、文本域自动高度与弹窗样式优化 ([88a6785](https://github.com/NateScarlet/image-funnel/commit/88a67853076a78b309e8dfde69929467434b5c22))
* **ImageGrid:** 通过 URL hash 实现图片查看器状态持久化 ([57a5017](https://github.com/NateScarlet/image-funnel/commit/57a501735e9f8e4ded8b081c7cdbd81207652ad4))
* Memo 废弃 title，新增 relPath 提供语义明确的笔记路径 ([cb9529b](https://github.com/NateScarlet/image-funnel/commit/cb9529b6eb6a55365da24ed105d81387a8bf2d4f))
* MemoEditorDialog 打开时自动聚焦到输入框 ([e240304](https://github.com/NateScarlet/image-funnel/commit/e240304f82ef0edbdea116c08cd3a446fa3fbe4f))
* prevent duplicate disk scans when scanning the same directory concurrently ([daa3b03](https://github.com/NateScarlet/image-funnel/commit/daa3b035bd4ca243fb12f09e151b624bbd28ace5))
* support manual refresh for local image list when metadata changes mismatch active filters ([c778aa3](https://github.com/NateScarlet/image-funnel/commit/c778aa315266dea5899d1b74571506d2a1874578))
* support reverse image preloading in image viewer ([87d52f7](https://github.com/NateScarlet/image-funnel/commit/87d52f731b2b5ccb29a19a09f42ef9a6b6bd4741))
* supports copy image absolute path ([28371fe](https://github.com/NateScarlet/image-funnel/commit/28371fee2344d87abd4953f83a52bb6c7a957fd6))
* supports early finish ([7a16b80](https://github.com/NateScarlet/image-funnel/commit/7a16b8063c320cd25a20c039e42a58239b225678))
* supports markdown memo ([7cc193a](https://github.com/NateScarlet/image-funnel/commit/7cc193ae247eaeab3ca72e3ad84c2008ef208cf8))
* 乱序标记图片时不再报错或移动队列位置 ([8fee85d](https://github.com/NateScarlet/image-funnel/commit/8fee85d5ce0968d3bff247d7928a4cd12d11e755))
* 优化顶部导航面包屑为多级级联跳转样式并提取为独立组件 ([87727d2](https://github.com/NateScarlet/image-funnel/commit/87727d23523b3fe90277bab8e85cf01c0c968700))
* 允许用户选中并复制面包屑导航中的目录名称文本 ([7eeccfd](https://github.com/NateScarlet/image-funnel/commit/7eeccfd6bd74ba5e7c567cf9c11d9f4e69c3c57f))
* 允许过滤隐藏未评级图片过多的子目录并优化开关滑块的点击交互 ([532e4ec](https://github.com/NateScarlet/image-funnel/commit/532e4ec226df193a438a932832e455e04ffc063e))
* 在 MemoList 中支持打开关联图片查看器 ([1ed4e70](https://github.com/NateScarlet/image-funnel/commit/1ed4e707801e4c76c24199b7a90ff242eed7225f))
* 在图片网格组件加载最新数据时显示旋转加载指示器 ([af5e1bd](https://github.com/NateScarlet/image-funnel/commit/af5e1bd059c6bf25e64fa21ec20001d9294898ab))
* 在子目录网格中显示各目录的子目录数量 ([2deec52](https://github.com/NateScarlet/image-funnel/commit/2deec527a12f9572bd8f619d23b24561c70f445c))
* 增加快捷键动态上下文支持并过滤未启用的快捷键列表 ([53e3240](https://github.com/NateScarlet/image-funnel/commit/53e3240004a7b3b83b9f1c476f1ebd282aa0451f))
* 增加本地目录筛选功能，仅显示未评级图片超过指定数量的目录 ([8f57320](https://github.com/NateScarlet/image-funnel/commit/8f57320cfe9078350692f790517d05e7d7afc05f))
* 增强会话进度条以展示图片操作历史颜色 ([c2939c3](https://github.com/NateScarlet/image-funnel/commit/c2939c3828b7ebb2640b434f3a19573ade04ba76))
* 增强会话进度条以展示图片操作历史颜色 ([4df5212](https://github.com/NateScarlet/image-funnel/commit/4df52120661a16ed6136944e16308e4c92f3e855))
* 复制工作流和路径成功或失败时显示全局气泡通知 ([ea625c3](https://github.com/NateScarlet/image-funnel/commit/ea625c316e3b4ce50d4130d058c2210b887fa6bf))
* 子目录支持分页加载与条件过滤 ([32151fd](https://github.com/NateScarlet/image-funnel/commit/32151fdb4a9140eb2d043ffc093711462f759b8c))
* 子目录网格支持本地文本搜索 ([2553ac2](https://github.com/NateScarlet/image-funnel/commit/2553ac2951b7f6f03c0e25768fa976dd9a72a2ff))
* 实时从浏览页面中移除已删除的笔记和子目录 ([30e93bb](https://github.com/NateScarlet/image-funnel/commit/30e93bbaac338cd73b4b65ddee1682e2b0907efb))
* 按修改时间倒序展示图片并支持时间游标分页 ([acc4c8b](https://github.com/NateScarlet/image-funnel/commit/acc4c8ba92fb467d12dc596f5101727e1237d5aa))
* 搜索目录时忽略当前筛选条件并标记不匹配的条目 ([c153fd9](https://github.com/NateScarlet/image-funnel/commit/c153fd9e383d945060f9bde3522f3b954faa9b34))
* 支持从服务器查询当前目录的最新会话作为筛选配置的进一步回退 ([b9a8fb2](https://github.com/NateScarlet/image-funnel/commit/b9a8fb2bfb12c9859e9f83e392678172ac409da2))
* 支持在图片查看器中切换并加载原图 ([227e40f](https://github.com/NateScarlet/image-funnel/commit/227e40f287cceadfdbd39488311eae184b48c3f8))
* 支持在图片查看器中即时设置与显示图片的 XMP 标签 ([9061c6f](https://github.com/NateScarlet/image-funnel/commit/9061c6f8f4013e75ce1abfff1d99ed6c4c3bbf87))
* 支持在图片浏览页面为各个目录分别记忆筛选状态 ([364fd3e](https://github.com/NateScarlet/image-funnel/commit/364fd3e7f38f6a6db5498b931066e9392ac71cda))
* 支持在大图查看器中使用 Home 和 End 键快速切换首尾图片 ([23c84b2](https://github.com/NateScarlet/image-funnel/commit/23c84b2f4dd893df57d81cb12c3dca866aae0303))
* 支持在子目录网格中通过服务端进行名称模糊搜索筛选 ([4131875](https://github.com/NateScarlet/image-funnel/commit/4131875ba49e10ba970db62d90d92b5f12ab0021))
* 支持在应用中直接浏览本地目录图片并即时更新元数据 ([c52f415](https://github.com/NateScarlet/image-funnel/commit/c52f4151b575503e6f88c494144d508ee7374d63))
* 支持在目录中列表展示 Markdown 笔记并提供一键切换隐藏状态功能 ([da1f7ee](https://github.com/NateScarlet/image-funnel/commit/da1f7eede010bc661e7b2248a145733c494a3c16))
* 支持在移动端使用物理返回键或手势返回关闭弹窗 ([4f169f3](https://github.com/NateScarlet/image-funnel/commit/4f169f36f1711297b85fb4bae384f72c7f0ce6e0))
* 支持在笔记列表中直接创建新笔记 ([a7205f5](https://github.com/NateScarlet/image-funnel/commit/a7205f5eba2b55d51f479f4ec7a3a05a00b5dbee))
* 支持在资源管理器中打开当前目录与定位图片，并提供独立打包构建脚本 ([37b2474](https://github.com/NateScarlet/image-funnel/commit/37b2474e46f6dadb12eb6863093de0c94f6414af))
* 支持对多张图片执行批量移动、评星和设置颜色标签的操作 ([bbc0f23](https://github.com/NateScarlet/image-funnel/commit/bbc0f23fe841ce12f6df411d66d1d53e40681c8f))
* 支持将筛选匹配的图片及其伴随文件移动至当前目录的相对子路径下并在资源管理器中打开 ([d202a42](https://github.com/NateScarlet/image-funnel/commit/d202a42d93bc6435281f0b910e0ec98223ef44f5))
* 支持按问号展示全局快捷键帮助弹窗并统一底部按键样式 ([41d3e46](https://github.com/NateScarlet/image-funnel/commit/41d3e46481ff3c433eb89c111ad3c30b01d278f0))
* 支持文件或目录移走与重命名后在前端实时更新清除 ([61e7688](https://github.com/NateScarlet/image-funnel/commit/61e7688596a3baf0966765ebf6af5e59e470a81a))
* 支持直接切换上一个和下一个同级子目录 ([1b4b943](https://github.com/NateScarlet/image-funnel/commit/1b4b943d0cc8ebd5dd028510e29e21694e476b21))
* 支持自动记录每个目录上一次会话的筛选条件和保留目标设置，并在新建会话或自动进入下个目录时优先继承 ([b87528e](https://github.com/NateScarlet/image-funnel/commit/b87528e85f22c1ed0996b439435c927171a5b880))
* 支持通过 Backspace 快捷键返回上一级目录 ([b7fd262](https://github.com/NateScarlet/image-funnel/commit/b7fd262f5c6722bc33c808f549bfd5361af83aab))
* 支持通过快捷键复制图片绝对路径 ([1c98e4b](https://github.com/NateScarlet/image-funnel/commit/1c98e4b02bd126ad7e2facc1ac1309bfb9c30602))
* 有活跃写入操作时自动阻止 Windows 系统休眠 ([67584ec](https://github.com/NateScarlet/image-funnel/commit/67584ecccbe2a43ee7def012238c864edd3693d0))
* 未评级图片筛选器改为带开关的样式并显示匹配目录数 ([77839ca](https://github.com/NateScarlet/image-funnel/commit/77839cabd1e80b4ea3a949862753cb7ab0b021bc))
* 添加 ComfyUI 工作流支持 ([973d040](https://github.com/NateScarlet/image-funnel/commit/973d040e25c8774f61d36f1407c865b46f8deb7c))
* 统一并在图片查看器中内置图片笔记编辑功能 ([080909c](https://github.com/NateScarlet/image-funnel/commit/080909c4009a3f270f7704a40333246fea3b66cf))
* 统一浏览视图各个区域的加载行为，都支持无限加载和底部加载按钮 ([2d67720](https://github.com/NateScarlet/image-funnel/commit/2d67720a6b7f19ab8a2817880c56b0b62c12f6df))
* 统一浏览页面的区域样式并展示上次会话的更新时间 ([a6adb04](https://github.com/NateScarlet/image-funnel/commit/a6adb04d865c36046fca0b3829f5e677908c54e5))
* 评分过滤器支持记住上次选择的操作模式 ([da00eed](https://github.com/NateScarlet/image-funnel/commit/da00eedf7530c0537b71860838b8c7c935ffffbe))
* 调整图片查看器的复制快捷键行为并新增复制路径快捷键 ([ab9f248](https://github.com/NateScarlet/image-funnel/commit/ab9f248d4ee5666f59ff45975b0348e5ae9ae11e))
* 调整清除筛选按钮的外观和位置并仅在有激活过滤器时显示 ([7e4bfb8](https://github.com/NateScarlet/image-funnel/commit/7e4bfb87284418b5626a017c693943dc9ae2d219))
* 调整评星快捷键为 Ctrl+0~5 与小键盘 0-5 并禁用单纯数字键 ([930113c](https://github.com/NateScarlet/image-funnel/commit/930113c48715b5bab0e041c6e2f8a1ec778a031d))
* 适配模态框在桌面端居中显示 ([4f6a14c](https://github.com/NateScarlet/image-funnel/commit/4f6a14c62c797425985b9535d91ada9dd456582a))
* 限制浏览视图各区域网格和列表的最大高度并支持溢出滚动，同时保持工具栏固定 ([e14bd13](https://github.com/NateScarlet/image-funnel/commit/e14bd13fbbf1fa978bdb6f79aef2cb70471c4a85))
* 首屏加载应用时显示品牌徽标与进度提示动画 ([4f75ee1](https://github.com/NateScarlet/image-funnel/commit/4f75ee135c504b2a2f291e915d4338396bcb35b7))


### Bug Fixes

* align filter bar heights and fix rating tooltip placement ([f56da07](https://github.com/NateScarlet/image-funnel/commit/f56da07a92ec8f329528b3d3a1e9a235c4c7df3e))
* directory item loading indicator stays spinning until entire list completes refetching ([22c9886](https://github.com/NateScarlet/image-funnel/commit/22c9886e59d17dc6ed0f50a34dc23b139300d80f))
* directory stats background loading blocks UI infinite scroll ([8cb9e1f](https://github.com/NateScarlet/image-funnel/commit/8cb9e1f8a2a01a95c98d4bceb388c96237db889c))
* directory stats loading states are not shown immediately for all pending directories ([cde39e3](https://github.com/NateScarlet/image-funnel/commit/cde39e342f9b99a3464fde60a66f9bd2c8726939))
* empty array in filter fields should matches nothing instead of bypass filtering ([a0497d9](https://github.com/NateScarlet/image-funnel/commit/a0497d9a28ad9f7bcafee6f904aee86c1e56a99b))
* infinite DirectoryStats queries in directory selector ([d0ad5e5](https://github.com/NateScarlet/image-funnel/commit/d0ad5e55616dc33e8f588320a6b28fb689995756))
* **memo:** inconsistent id ([608be2b](https://github.com/NateScarlet/image-funnel/commit/608be2b6a519f04c6e8688759325ac8596fd94e9))
* **memo:** inconsistent title ([4921a6b](https://github.com/NateScarlet/image-funnel/commit/4921a6b67f177070f75dc66383deb810a9d39616))
* search input disappears when filtered items count drops below six during typing ([179bb91](https://github.com/NateScarlet/image-funnel/commit/179bb91b7d9249a404019f83d0a847132fff0712))
* search query persists when switching directories ([f698bfe](https://github.com/NateScarlet/image-funnel/commit/f698bfea06ec2217ed4f565665289084bbe55386))
* **session:** should allow set full filters on session update ([769feea](https://github.com/NateScarlet/image-funnel/commit/769feeaefd1a4c295d091dc32abd6c704e8a3640))
* **session:** 乱序标记不应限于当前轮 ([ae43c3b](https://github.com/NateScarlet/image-funnel/commit/ae43c3b5c03e017d85bb2aac92f9de1a8f59e543))
* should handle cache update for live created image ([35079c2](https://github.com/NateScarlet/image-funnel/commit/35079c2fc0ca104a0d9d00318adb24b30eaaac84))
* should isolate app element ([a6e7bbe](https://github.com/NateScarlet/image-funnel/commit/a6e7bbe14cd4f2b4fecbeb63e68117a34893a73b))
* should make dir before image save ([dcce3b6](https://github.com/NateScarlet/image-funnel/commit/dcce3b65c99f18d65f772e8d4c9235289d1bfb87))
* should not send raw image when can not process ([f5c6049](https://github.com/NateScarlet/image-funnel/commit/f5c6049a652ffb65bed57c2134cc13c7480739d5))
* should prevent current directory from being added to the subdirectory grid upon self change ([df45f03](https://github.com/NateScarlet/image-funnel/commit/df45f037dd13d3b6657f0500b32bec582847545a))
* some directories cannot be seen or searched in directory selector ([1d4b0f6](https://github.com/NateScarlet/image-funnel/commit/1d4b0f6126ddaef2398e7f36624a669b1a5716f7))
* updateMemo mutation returns null after deleting memo ([010f8e1](https://github.com/NateScarlet/image-funnel/commit/010f8e1568dcad3b6797f5de120800861fb242a9))
* VS Code ESLint 插件在工作区根目录下对 Vue 文件报 TSConfig 解析错误 ([d8425c3](https://github.com/NateScarlet/image-funnel/commit/d8425c3770f0923209df1cdb5ac78d990fe2cf83))
* Windows 下文件监控器的内存对齐错误 (998) ([2257dc9](https://github.com/NateScarlet/image-funnel/commit/2257dc910e3301d0f5004adb428ad042dda73e92))
* wrong image memo path ([83e4e04](https://github.com/NateScarlet/image-funnel/commit/83e4e047d91732a2e27eb49b465aced42dc54a99))
* 乱序标记在队列末尾时应触发新一轮筛选 ([07f3405](https://github.com/NateScarlet/image-funnel/commit/07f3405b758f22bf81ac8dbd79a710082b429e36))
* 保证浏览页面滚动时导航栏始终位于最上层，避免被内容遮挡 ([59c80a0](https://github.com/NateScarlet/image-funnel/commit/59c80a015209dde9906d46aae6ff4f342d86eb26))
* 修复对话框点击遮罩关闭时无动画 ([1fd1013](https://github.com/NateScarlet/image-funnel/commit/1fd10130afacc5934faad31fb959b854005291b4))
* 关闭图片查看器后 URL 哈希值被意外还原 ([4ec58d7](https://github.com/NateScarlet/image-funnel/commit/4ec58d759513031543229dfbaf4ad86816370527))
* 切换图片后复制工作流仍然得到上一张图片的数据 ([008635b](https://github.com/NateScarlet/image-funnel/commit/008635b0c23dbec182e22ffa46b7c228da7216b1))
* 图片二进制数据被错误载入为笔记文本 ([e65def3](https://github.com/NateScarlet/image-funnel/commit/e65def3056e7af0b878be04a80badc5739b79603))
* 实时创建图片的修改状态未能同步到大图查看器 ([46cb740](https://github.com/NateScarlet/image-funnel/commit/46cb740437346fa4d2803e34f57e848a16025094))
* 实时更新图片列表时全屏查看器中的当前图片不再错位 ([757f939](https://github.com/NateScarlet/image-funnel/commit/757f9390d0ddd35026096353bb76d50b2549d8a3))
* 并发加载同一转码图片时出现的数据流读取冲突 ([7a255de](https://github.com/NateScarlet/image-funnel/commit/7a255def7994814d7c35703f12fa21f159e09e99))
* 应图像未完全加载时在待移动数量前显示大于号 ([67a2d5b](https://github.com/NateScarlet/image-funnel/commit/67a2d5bb511f810a8585e5dbe2d3b7f90ad71baa))
* 拖放下载图片时保留原本的文件名 ([18cf61c](https://github.com/NateScarlet/image-funnel/commit/18cf61cef37e25dbff2ff2400062cd0c04226337))
* 接口返回重复错误信息 ([ce77daa](https://github.com/NateScarlet/image-funnel/commit/ce77daa0f4064919e704388531619433af56ba4a))
* 提交后撤销重新标记时保留数量减少且无法二次提交 ([ca6f1e9](https://github.com/NateScarlet/image-funnel/commit/ca6f1e9622bda150c1f65b47f586e65fdf60a20d))
* 提高 Windows 系统下文件监控的稳定性与内存安全性 ([1efd7ca](https://github.com/NateScarlet/image-funnel/commit/1efd7ca469d903b319c4a25c03c016047337b92c))
* 提高快捷键帮助模态框的视觉层级并按功能模块分组展示 ([2db039f](https://github.com/NateScarlet/image-funnel/commit/2db039f02decf65b57ca74567da1f2afd5a58772))
* 支持正确打开包含逗号的文件夹路径 ([afdd90b](https://github.com/NateScarlet/image-funnel/commit/afdd90b1aabe0fa43c9804181aa5b4541d209fcd))
* 模态框打开时自动取消文本选中以防止复制冲突 ([20a2898](https://github.com/NateScarlet/image-funnel/commit/20a289805d25144c7ea5d763b1d1e7b22fd4d164))
* 目录变更订阅引发的前端面包屑导航无限递归渲染 ([476ff77](https://github.com/NateScarlet/image-funnel/commit/476ff77898d896a5c5b8734992aafcd8c318ef83))
* 目录选择器中包含未评级图片的目录显示不全 ([a053cc2](https://github.com/NateScarlet/image-funnel/commit/a053cc284f7f75c79df29d394e9b47d14cb8ff84))
* 移动端导航栏适配 ([b022806](https://github.com/NateScarlet/image-funnel/commit/b022806ac0e0e1c9042e4f2cc15dc0db5e16ab64))
* 移动端点击修改会话或提交表单后弹窗自动收起 ([1286ea2](https://github.com/NateScarlet/image-funnel/commit/1286ea2dd1008dc2095c4485e4a82beda6315e61))
* 移除图片查看器工具栏尾部多余的分割线 ([8b279c5](https://github.com/NateScarlet/image-funnel/commit/8b279c5ed9a7a1a45bdfb6e9009cf69b43f5114c))
* 统一筛选开关样式并消除浏览页面中隐藏笔记切换控制的歧义 ([cf2119d](https://github.com/NateScarlet/image-funnel/commit/cf2119d2fe8878170de1ab778c8d371622a69340))
* 缓存文件被并发清理时，Open 方法可能返回错误而非静默回退 ([89a9ec9](https://github.com/NateScarlet/image-funnel/commit/89a9ec9f7dee0d53a755ed8ccca7ec2e805fa575))
* 解决图片未写入完成即转码时导致的 unexpected end-of-file 报错 ([aa98911](https://github.com/NateScarlet/image-funnel/commit/aa98911a86ea0ccf5d21ec92ff5725815d7fc58d))
* 解决子目录搜索时输入框因加载状态导致失焦及布局抖动 ([5309b76](https://github.com/NateScarlet/image-funnel/commit/5309b7609882ed88e221ca23cc9e40ad82f828e3))
* 解决并发修改 XMP 文件时的冲突失败错误 ([77f8952](https://github.com/NateScarlet/image-funnel/commit/77f8952a03e384346a09720a48a01cea5a033402))
* 评分过滤器操作符模式完全由用户切换而不再受数值改变影响 ([5b93313](https://github.com/NateScarlet/image-funnel/commit/5b933135816cc79e752c9e5e8367cdc8a106eaa2))
* 调整目录浏览器顶部返回上一级按钮在不可用时的表现，避免布局抖动 ([7c4a0c1](https://github.com/NateScarlet/image-funnel/commit/7c4a0c14d50224ffe07c13ab4eaf376bfe8c6fd9))
* 避免在 Windows 上修改图片元数据时因文件冲突而报错 ([cf3e956](https://github.com/NateScarlet/image-funnel/commit/cf3e956e26e2837c1cb81463adf4594e2a71fa03))
* 避免在图片文件写入或更新时因修改时间变化导致显示重复图片 ([73c9268](https://github.com/NateScarlet/image-funnel/commit/73c9268d1f8d4f7c4c1fae8b685fb56fb82dc885))
* 避免在非图片或无效文件触发变更事件时导致服务崩溃 ([b0857e9](https://github.com/NateScarlet/image-funnel/commit/b0857e95213c05bd1cacfbc4ce375d8d3967766a))
* 避免清理缓存时因文件不存在输出无意义的错误日志 ([b147e61](https://github.com/NateScarlet/image-funnel/commit/b147e61550d7f075eb8ef858cce2273d5dc43f9b))
* 防止客户端取消订阅时服务器因空指针异常退出 ([aa50023](https://github.com/NateScarlet/image-funnel/commit/aa50023540e21a1c7f089a1b2e8f0975856296ad))
* 限制图片加载时的模糊遮罩范围至视口区域，避免工具栏被模糊 ([f26b762](https://github.com/NateScarlet/image-funnel/commit/f26b76221377d893200d3e69afadb9b6abada6b2))


### Performance Improvements

* 优化会话进度条性能，使用渐变背景替代大量 DOM 元素 ([ed70e77](https://github.com/NateScarlet/image-funnel/commit/ed70e7721f19e144e3903b416b11b531b45f4cfb))

## [1.4.1](https://github.com/NateScarlet/image-funnel/compare/v1.4.0...v1.4.1) (2026-02-24)


### Bug Fixes

* **session:** 快速执行撤销和标记时部分图片丢失处理状态 ([ac03c23](https://github.com/NateScarlet/image-funnel/commit/ac03c23b3a935183ac0a42c4aac6a5f7136e2ab1))
* undo 后 commit 触发文件监听器错误地将已操作图片移出队列 ([554f3e8](https://github.com/NateScarlet/image-funnel/commit/554f3e8188ed18adc6b680ca11e969f80256e1b3))
* 其它过滤条件生效导致可选目录为空时应隐藏搜索框 ([5aceca9](https://github.com/NateScarlet/image-funnel/commit/5aceca947acc2555ed5dab0d0902856d5d2f6654))
* 后端运行期间允许在外部正常删除被监控的文件夹目录 ([fc1099f](https://github.com/NateScarlet/image-funnel/commit/fc1099f615ce454e9ae518e99c660e8fb365dbca))
* 并发 map 读写导致的 fatal error ([5059a12](https://github.com/NateScarlet/image-funnel/commit/5059a122e9d33db3233f9f19641fb46508603dce))
* 目录文件变更时统计信息不更新 ([4192ce5](https://github.com/NateScarlet/image-funnel/commit/4192ce530f8a76b3915daa5817be38cb1b9ff18e))


### Performance Improvements

* **frontend:** 优化选择器在面对海量目录时的浏览流畅度 ([6750ff9](https://github.com/NateScarlet/image-funnel/commit/6750ff9d72113553a5444d8a99a0f2858ed819d6))
* 优化目录统计查询的性能 ([85efdc5](https://github.com/NateScarlet/image-funnel/commit/85efdc51962b39e78d2ac2834f25faf067659549))

## [1.4.0](https://github.com/NateScarlet/image-funnel/compare/v1.3.0...v1.4.0) (2026-02-10)


### Features

* improve directory search experience in selector ([f58781b](https://github.com/NateScarlet/image-funnel/commit/f58781b25b4bccce8f6a47e5bdf4a9fbc08dc81d))
* increase image thumbnail  quality ([1ab5983](https://github.com/NateScarlet/image-funnel/commit/1ab5983df7c93cdef445d1451612b4a9ef215532))
* limit concurrent ImageMagick process execution to prevent memory issues ([2d2233c](https://github.com/NateScarlet/image-funnel/commit/2d2233c7db350fef5dd9a318655dee2f0f9435ca))
* 为预设名称添加 Emoji 前缀以便区分 ([314a0a1](https://github.com/NateScarlet/image-funnel/commit/314a0a1232f5f14c4a17f246cc8c719ee795a03c))
* 将更新会话弹窗的预设选择改为平铺卡片样式 ([b3590b3](https://github.com/NateScarlet/image-funnel/commit/b3590b3a005e1996f4c43f646924c20d7b16276d))


### Bug Fixes

* fix directory index not updated during session cleanup ([5f902be](https://github.com/NateScarlet/image-funnel/commit/5f902bef99473c7b77011b2f93a28bb6469261ef))
* ImageViewer 双指缩放/位移时误触发 SessionView 标记手势 ([3de7c70](https://github.com/NateScarlet/image-funnel/commit/3de7c70c7510b201920474bb7066711e88edff28))
* unexpected value reset on customize preset ([0f6b75b](https://github.com/NateScarlet/image-funnel/commit/0f6b75ba5ad0ce663487d2d3b618c8fb3a5aee43))
* 动图缩放时产生黑色噪点和透明度异常 ([950e1df](https://github.com/NateScarlet/image-funnel/commit/950e1dfd9f388c33f14019091dea396b523102cf))
* 双指缩放后视图跳动 ([792ca1e](https://github.com/NateScarlet/image-funnel/commit/792ca1e3cdaba0143e9cfd296c241dab13e03230))
* 应允许在完成界面垂直滚动 ([9151a5f](https://github.com/NateScarlet/image-funnel/commit/9151a5f63218ec4d6d530dbd20a16dd78c723eb0))
* 应当仅返回和保存符合当前会话筛选条件的图片操作 ([80bc561](https://github.com/NateScarlet/image-funnel/commit/80bc561ce89f8890d4dbbe7262bc945578e28816))
* 提交后应立即更新会话状态以防止操作遗漏 ([715169b](https://github.com/NateScarlet/image-funnel/commit/715169b1189d837fb0d26ce84a1814440ce3f489))
* 提交图片时正确报告文件加载错误和外部修改 ([e593f87](https://github.com/NateScarlet/image-funnel/commit/e593f870b2bee953a97a23920080b14cebff4e52))


### Performance Improvements

* optimize session image lookup and updates ([4f0159d](https://github.com/NateScarlet/image-funnel/commit/4f0159d3fdfa721df17f5f22fdb71f24eb9819fc))
* 并发预加载加载多张后续图片 ([0a627ed](https://github.com/NateScarlet/image-funnel/commit/0a627ed5f9f2db9fe36baf3c02a95c3be728a7b0))

## [1.3.0](https://github.com/NateScarlet/image-funnel/compare/v1.2.1...v1.3.0) (2026-02-05)


### Features

* add confirmation dialog when submitting with 0 kept images in CompletedView ([99825ea](https://github.com/NateScarlet/image-funnel/commit/99825eadb49554eab6306c3aa951cc2405e1f447))
* **frontend:** read directory stats from persisted cache ([b13e176](https://github.com/NateScarlet/image-funnel/commit/b13e1768f0ce00f9ec9cb9e47f90fd4eee55b850))
* lazy load directory cover image ([914e3ff](https://github.com/NateScarlet/image-funnel/commit/914e3ffba9f8ec4295d44804ab374f95a99b3a1c))
* support directory search in DirectorySelector ([c8f339c](https://github.com/NateScarlet/image-funnel/commit/c8f339cca5707abf7d3c8ee6f70fd98181c2d3ed))
* 增加目录列表加载提示 ([8584696](https://github.com/NateScarlet/image-funnel/commit/858469656491f611635f2faf00b742d95cf40109))


### Bug Fixes

* **session:** 更新筛选条件导致保留的图片不再符合条件后无法完成会话 ([54627ad](https://github.com/NateScarlet/image-funnel/commit/54627ad5eebc6565a876de949734f315538c345d))
* 禁用页面缩放以避免手势冲突 ([5038cf5](https://github.com/NateScarlet/image-funnel/commit/5038cf5e44b2b86bbf74548caf77d0403a2d52bb))


### Performance Improvements

* configure batch http group key ([3934b52](https://github.com/NateScarlet/image-funnel/commit/3934b522190c6ab04cfbbf3e86e18b5a9feef4e2))
* use persisted query with websocket message ([ae4861b](https://github.com/NateScarlet/image-funnel/commit/ae4861b059d8deca127d2d5d0225259eb8b199eb))
* 优化引用稳定性，避免 UI 不必要的重绘 ([3a3e1ca](https://github.com/NateScarlet/image-funnel/commit/3a3e1ca55b772aa9ff1aed7e7c82c09f8f838651))
* 将 Apollo Client 缓存持久化迁移到 IndexedDB ([26ea9cc](https://github.com/NateScarlet/image-funnel/commit/26ea9cc5ef1d4d7431c60baec234361c2515d83c))

## [1.2.1](https://github.com/NateScarlet/image-funnel/compare/v1.2.0...v1.2.1) (2026-02-03)


### Bug Fixes

* **session:** 过滤条件更新后新图片未被正确写入 ([44bbf0c](https://github.com/NateScarlet/image-funnel/commit/44bbf0cd062ae98bd5b8e6719e3e7e4df5682b0c))

## [1.2.0](https://github.com/NateScarlet/image-funnel/compare/v1.1.0...v1.2.0) (2026-02-02)


### Features

* support undo gesture in completed session view ([baeac7b](https://github.com/NateScarlet/image-funnel/commit/baeac7b42fbb67ee294db13faf318f14036f25b4))
* use raw image url for downloads ([5fd78e0](https://github.com/NateScarlet/image-funnel/commit/5fd78e0e604643f2d37f10fc94f579fe3b9fe583))
* 首页目录选中状态同步 URL 参数，优化会话页返回逻辑 ([5b9cd48](https://github.com/NateScarlet/image-funnel/commit/5b9cd481ecd037b3dbcd487f43dd1b39226b0cb8))


### Bug Fixes

* should dispose graphql query on scope dispose ([5a1d4e7](https://github.com/NateScarlet/image-funnel/commit/5a1d4e7765d4f17e69fdf71b2a58165dd02fbd75))
* 子目录下新生成文件不触发界面更新 ([5efb558](https://github.com/NateScarlet/image-funnel/commit/5efb558e37b476efa91c60ae45c727ef28b3ae03))

## [1.1.0](https://github.com/NateScarlet/image-funnel/compare/v1.0.1...v1.1.0) (2026-01-31)


### Features

* 优化下一轮图片排序逻辑避免连续重复 ([657c287](https://github.com/NateScarlet/image-funnel/commit/657c287bebd927d504704de78bc221be183fee0f))
* 在图片查看器中添加会话进度条 ([f9e2a02](https://github.com/NateScarlet/image-funnel/commit/f9e2a02da57c2317d68af4a7d66b6ffceb1f19f3))


### Bug Fixes

* 修复 SessionView 中 sessionId 响应式丢失问题 ([57cce71](https://github.com/NateScarlet/image-funnel/commit/57cce71f016b0794c5aba6529ed94f59f032ed3c))

## [1.0.1](https://github.com/NateScarlet/image-funnel/compare/v1.0.0...v1.0.1) (2026-01-30)


### Bug Fixes

* update broken ImageMagick download URL in launcher script ([328827e](https://github.com/NateScarlet/image-funnel/commit/328827e7fc121def7f34a5d2048bb53f92c7a88f))

## 1.0.0 (2026-01-30)


### Features

* add command log ([e66b258](https://github.com/NateScarlet/image-funnel/commit/e66b2580ce30f1e90fd84b6f13d241cda6b0b2b9))
* add GraphQL batching support for improved performance ([be17a80](https://github.com/NateScarlet/image-funnel/commit/be17a8087dab17539443e194fd2617be5ead7bc8))
* add undo support to completed view and preserve viewer state ([31c7a56](https://github.com/NateScarlet/image-funnel/commit/31c7a5660acf5503c0a2b3d3aeb386132ca8a5ef))
* CORS support for http service ([b76e9a8](https://github.com/NateScarlet/image-funnel/commit/b76e9a864a1cd3225e8832ee7825b931be0f18f3))
* display current rating in image viewer toolbar ([77b719f](https://github.com/NateScarlet/image-funnel/commit/77b719f61e9bb89ba14ea7e9d8f5fcd54b167eb9))
* **frontend:** add APQ (Automatic Persisted Queries) support for GraphQL client ([9b64d10](https://github.com/NateScarlet/image-funnel/commit/9b64d10389afacf0dad684a47b7baab32ea26fdc))
* generate and load app secret in run.ps1 ([ebb71f4](https://github.com/NateScarlet/image-funnel/commit/ebb71f4b069d73d6bf2c75b66b79fb7a7ef8152e))
* **graphql:** 添加错误处理链路以显示GraphQL和网络错误 ([898dc5f](https://github.com/NateScarlet/image-funnel/commit/898dc5f081d25b85cf1c0b6fd90f021328aea01b))
* handle file update and delete events in session service ([6e3424f](https://github.com/NateScarlet/image-funnel/commit/6e3424f5e22c99e90ac91490b144a1b507de6820))
* **ImageViewer:** 优化响应式布局和全屏状态下的信息显示 ([ac340c3](https://github.com/NateScarlet/image-funnel/commit/ac340c3b78509a085e056fdc704b6490cd3d906f))
* **ImageViewer:** 重构图片查看器并添加缩放控制栏 ([c1f8f21](https://github.com/NateScarlet/image-funnel/commit/c1f8f21096b599bbd1ae637a02f279842da9c081))
* implement duration tracking for image review ([3776667](https://github.com/NateScarlet/image-funnel/commit/377666722ef43484a17bc27447902ea518afd586))
* implement Node interface for Directory and add node query ([8069206](https://github.com/NateScarlet/image-funnel/commit/80692068729294b1274b59b2bbc1345f1069142d))
* remove preset xmp field ([689ae5e](https://github.com/NateScarlet/image-funnel/commit/689ae5e85e2df3c2ac619f3a171bafcb6dce5d65))
* remove unnecessary check on signed url ([8cadc83](https://github.com/NateScarlet/image-funnel/commit/8cadc83a16a502ea3ec163fcd95c6395cf95aed6))
* rename PENDING to SHELVE and exclude from next round ([dca1bd6](https://github.com/NateScarlet/image-funnel/commit/dca1bd6e5ca36b619d3ac9c70400325c3412a3fe))
* **SessionView:** 添加移动端响应式布局和菜单功能 ([92c7c0f](https://github.com/NateScarlet/image-funnel/commit/92c7c0fceb9ae3dfc43423dd4e2dc062685ec16b))
* **session:** 支持中途修改预设 ([2a0f3bf](https://github.com/NateScarlet/image-funnel/commit/2a0f3bf0e6d611bb9fac0aa1d30533fd489c28af))
* **session:** 添加图片评分过滤功能 ([44082a9](https://github.com/NateScarlet/image-funnel/commit/44082a9d8bf182ec4398555425d96337161a7917))
* **session:** 添加支持回退到上一轮的功能 ([28d54cb](https://github.com/NateScarlet/image-funnel/commit/28d54cbc1791cab8e07983bd1e59d2350d230ddb))
* **ui:** 使用 secondary 颜色替换主要按钮颜色 ([35cd67d](https://github.com/NateScarlet/image-funnel/commit/35cd67d1d6770f95dff1f156135ecb70a274903a))
* **util:** 实现原子文件保存功能并应用于xmp模块 ([e205369](https://github.com/NateScarlet/image-funnel/commit/e205369114fa99b54af52bc71575cd7f2eae7300))
* **web:** show commit form directly after complete ([c44cc0f](https://github.com/NateScarlet/image-funnel/commit/c44cc0f2096b3b10e1e59f376b8eb3f595cb30da))
* 为按钮添加加载状态和动画效果 ([6ab5a93](https://github.com/NateScarlet/image-funnel/commit/6ab5a9324dab6d36c1675b35e0080fbdcb9e0730))
* 主动按顺序预加载后续图片 ([5cd7dcb](https://github.com/NateScarlet/image-funnel/commit/5cd7dcbfe512cc6b98d09c3c2e06c9f0d6ca0d5b))
* 优化保留图片列表的移动端交互体验 ([4a5cef8](https://github.com/NateScarlet/image-funnel/commit/4a5cef84a1feb97eacdecf1c2cf7b8d8abe7d0a4))
* 优化图片加载提示的响应速度 ([7fe140f](https://github.com/NateScarlet/image-funnel/commit/7fe140fb6e0bb2e2c467509121ed27585d73a1cb))
* 优化图片双指缩放体验 ([4eab430](https://github.com/NateScarlet/image-funnel/commit/4eab430a477ffbf759f324d9bd12326e2dd16bc4))
* 优化图片查看交互与操作限制 ([23dab84](https://github.com/NateScarlet/image-funnel/commit/23dab844990eda1a067ba91b912a8e57b2567812))
* 修改预设表单默认选中最后选择的会话 ([2ed909b](https://github.com/NateScarlet/image-funnel/commit/2ed909b95f8accd6d6f9b43fa00494f5d9ea261a))
* 允许在会话完成页面通过顶部按钮直接提交 ([758ad77](https://github.com/NateScarlet/image-funnel/commit/758ad7721bbe2edee78e3c31636641b0f35aebb5))
* **全屏:** 添加全屏渲染元素支持 ([f1f9172](https://github.com/NateScarlet/image-funnel/commit/f1f91729f893608f9f266fb296fd5692484d4f71))
* **前端:** 将撤销按钮移动到顶部操作栏 ([64a2d9e](https://github.com/NateScarlet/image-funnel/commit/64a2d9e0764e3a112fb9131b01a4823a8c78281a))
* **前端:** 调整目标数量输入框位置并保持功能不变 ([94b3893](https://github.com/NateScarlet/image-funnel/commit/94b3893e5d3ef5976de2a4d6a545726309c190c9))
* 只在慢加载时显示提示 ([a70d855](https://github.com/NateScarlet/image-funnel/commit/a70d85592e6ce98ece620bf1a01e8400fcf6debe))
* **图片信息:** 添加图片修改时间字段并在前端显示 ([5179644](https://github.com/NateScarlet/image-funnel/commit/517964428729d13e197703706c19b060170d2a5a))
* **图片查看器:** 添加全屏功能支持 ([3663d0e](https://github.com/NateScarlet/image-funnel/commit/3663d0e86c3bd9467c9a13660ad0a398b8c371bb))
* **图片查看器:** 添加图片缩放、拖拽和触摸手势支持 ([d4f29aa](https://github.com/NateScarlet/image-funnel/commit/d4f29aaa4612adb08f83d91681ba954f1cb8adbb))
* 在 SessionHeader 添加返回首页图标 ([24a55ac](https://github.com/NateScarlet/image-funnel/commit/24a55acbc7dfed2fd2d81127ac4d5ff6479a0624))
* 在完成页面添加下一个目录的显示 ([a270491](https://github.com/NateScarlet/image-funnel/commit/a270491d3cfd83f76cad0b634badc0a92b739cb0))
* 增加下一张图片的预载功能 (使用 link prefetch) ([563048b](https://github.com/NateScarlet/image-funnel/commit/563048be34261f46c9c687bb226e2eb2e3e1d44f))
* 实现 Apollo 缓存持久化 ([a923f13](https://github.com/NateScarlet/image-funnel/commit/a923f1309dd0daf628b88700983463cff6583ea4))
* 实现基于缩放级别的图片预加载功能 ([4ab2bca](https://github.com/NateScarlet/image-funnel/commit/4ab2bca8a6c4bac835d242a0a1364731aebbb3eb))
* 展示保留的图片 ([909043a](https://github.com/NateScarlet/image-funnel/commit/909043a5b2a6847c1696bc57bb44dbfe5936859d))
* 引入统一的错误处理机制 ([fcdd589](https://github.com/NateScarlet/image-funnel/commit/fcdd589519e231730c19ccf74d661fe861268863))
* 引入自定义ID类型替代字符串类型 ([d2f7ea9](https://github.com/NateScarlet/image-funnel/commit/d2f7ea92ea516f9fd8c5a8d0cb174bc5b9dd7bf3))
* 按缩放大小动态加载不同分辨率的图片 ([bebedea](https://github.com/NateScarlet/image-funnel/commit/bebedea8b65c978483064c89fe72db0462e28b41))
* 提交后如果无错误自动关闭结果显示 ([bfa05ad](https://github.com/NateScarlet/image-funnel/commit/bfa05adc22826a8248d62277dfbabbc22863937d))
* 支持图片缩放处理，节省带宽 ([3204384](https://github.com/NateScarlet/image-funnel/commit/3204384cef3c3e9932305283aa75d605071d339a))
* 支持实时添加新图片 ([a5dd064](https://github.com/NateScarlet/image-funnel/commit/a5dd064a61eb7546937ea7da0b1017232b89316e))
* 支持预加载多张图片以提升快速筛选体验 ([3979629](https://github.com/NateScarlet/image-funnel/commit/3979629c99d26e26be7986cff67bd859da4c51c9))
* 显示当前目录的统计信息 ([c5a04a6](https://github.com/NateScarlet/image-funnel/commit/c5a04a671621da846ee43a635d851fac538fd731))
* **构建:** 添加构建版本信息注入功能 ([e8e54fd](https://github.com/NateScarlet/image-funnel/commit/e8e54fdc41f65042f22b26adef2739663ef9446b))
* 添加内存会话仓库的自动清理机制 ([8297b8b](https://github.com/NateScarlet/image-funnel/commit/8297b8b0afb028439d581e0b3aa14e72811c9021))
* 添加前端实时订阅支持，实现 Session 自动更新 ([3552b57](https://github.com/NateScarlet/image-funnel/commit/3552b57a96aae15a6d2b3d6512e51d783e00ea43))
* 添加图片加载提示 ([d06f53f](https://github.com/NateScarlet/image-funnel/commit/d06f53f7835984f477c0d2f8771801209f9ce986))
* 添加目录筛选开关和达标提示 ([c56d69c](https://github.com/NateScarlet/image-funnel/commit/c56d69cca87b47738f224ffa94c28b87c8488cd8))
* **界面:** 为按钮添加图标以提升用户体验 ([ccdb8d8](https://github.com/NateScarlet/image-funnel/commit/ccdb8d889ee763142065c45670846f239bbb8c37))
* **目录服务:** 实现目录ID编码解码并增强路径验证 ([4a4bfef](https://github.com/NateScarlet/image-funnel/commit/4a4bfef0170ce57f8e867c350a0f8515b78a52a8))
* **目录:** 添加根目录标识字段 ([ecb086d](https://github.com/NateScarlet/image-funnel/commit/ecb086d4a923779aa7d079c1e015b3016e6c9f76))
* **目录:** 添加父目录ID支持以改进导航功能 ([9213118](https://github.com/NateScarlet/image-funnel/commit/9213118bfdfad7e9beb35eb9e1a71347d8cfc243))
* **目录:** 添加父目录ID支持并优化目录导航逻辑 ([7bcabeb](https://github.com/NateScarlet/image-funnel/commit/7bcabebbcdb18565107502b44e53d563565ab519))
* **目录评分:** 添加目录图片评分统计功能 ([cb8175a](https://github.com/NateScarlet/image-funnel/commit/cb8175a85dea30f10df61ae922254923fb4c8184))
* **目录选择:** 添加目录浏览和选择功能 ([d30fec0](https://github.com/NateScarlet/image-funnel/commit/d30fec018d0e9a09f9f328f442b230851a8f512d))
* **视图:** 在会话视图中添加图片处理统计信息显示 ([fc18bbf](https://github.com/NateScarlet/image-funnel/commit/fc18bbf2d90d9b8545108c405f1cf60e6f078d8a))
* 移动端菜单增加撤销按钮 ([c39ea52](https://github.com/NateScarlet/image-funnel/commit/c39ea52f5c37549cc2049ddc59394abe2e28190e))
* 移除放弃按钮，用户可直接使用浏览器导航功能 ([028ab2c](https://github.com/NateScarlet/image-funnel/commit/028ab2c40ab30db3c47668cc40775faaefa6955e))
* **组件:** 新增RatingIcon组件并替换现有星级显示 ([fd780c8](https://github.com/NateScarlet/image-funnel/commit/fd780c8dd1dfdb2f24d5bea85a0602ec283ee62c))
* 自动前往下一个未完成目录 ([8dc0c56](https://github.com/NateScarlet/image-funnel/commit/8dc0c567175f5178a17a66a0941d46dbbd74c4a9))
* 调整目录顺序 ([0842411](https://github.com/NateScarlet/image-funnel/commit/0842411eee0b791f12a94e68c45c0e21b47a02ce))
* **通知:** 实现全局通知系统 ([4b1dc58](https://github.com/NateScarlet/image-funnel/commit/4b1dc582a43277f5b0d4888cec15e13bc732711c))
* 限制同时显示的消息数量并添加清除所有按钮 ([fe898fa](https://github.com/NateScarlet/image-funnel/commit/fe898faa872e916bc9ef30152108e7b7f93766b2))
* **预设:** 添加目标保留数量字段并更新过滤逻辑 ([ff84724](https://github.com/NateScarlet/image-funnel/commit/ff847248f11dce71c04e1b5b613433350afb5497))
* **首页:** 添加日期格式化功能并更新目录显示 ([58c24bb](https://github.com/NateScarlet/image-funnel/commit/58c24bbc93b5723a783745ece5c8ef21871046d9))


### Bug Fixes

* can not persistent on root dir ([1bd4d18](https://github.com/NateScarlet/image-funnel/commit/1bd4d181bf718e57546531a566b91ee6c42d91b3))
* correct Microsoft Photo rating metadata mapping ([f8dd158](https://github.com/NateScarlet/image-funnel/commit/f8dd158974548304febfbc8932913ee549de038a))
* exclude shelved images from session stats completion logic ([c9b6671](https://github.com/NateScarlet/image-funnel/commit/c9b6671cd03606536cc892e23cab0b3270e89c9d))
* false positive when validate rel path ([a31ca50](https://github.com/NateScarlet/image-funnel/commit/a31ca50c94a23e227897c54fcb42e0536c02919c))
* **frontend:** 修正 SessionView 左右滑动的颜色与标签显示 ([560399f](https://github.com/NateScarlet/image-funnel/commit/560399fdc9f9208b433d0e189462aee3fd139a2b))
* **frontend:** 滑动不应优先于其他基本交互触发 ([4644e96](https://github.com/NateScarlet/image-funnel/commit/4644e96845e16eb9b4b0d28f059a708c4d51205a))
* **frontend:** 非安全环境下无法使用 apq ([7228d15](https://github.com/NateScarlet/image-funnel/commit/7228d15610066b95745e81a85b6d9e42b7a89df1))
* **graphql:** 防止重复显示相同的错误消息 ([cd04379](https://github.com/NateScarlet/image-funnel/commit/cd04379bd4b423fc8c3d2ce7cb21f329e2d32fe8))
* handle context cancellation in image processor ([9d5962b](https://github.com/NateScarlet/image-funnel/commit/9d5962b919f6394032ee335e68a3baef1b3535e2))
* handle storage format change ([b3d5f19](https://github.com/NateScarlet/image-funnel/commit/b3d5f19b154c5047696b017a8824c9683f520bb5))
* **HomeView:** 修复星级评分颜色显示问题 ([68ae5d7](https://github.com/NateScarlet/image-funnel/commit/68ae5d74384ea4d4bda00b1665099eeb47527258))
* **HomeView:** 修复评分图标选中状态显示问题 ([db27493](https://github.com/NateScarlet/image-funnel/commit/db274938b017872fd05b7ab7f73dcf3ad6d07c35))
* **HomeView:** 移除不必要的目录ID检查以简化创建条件 ([3c524e1](https://github.com/NateScarlet/image-funnel/commit/3c524e138d679d8b2d6cbce992f45ee68f75b92f))
* **HomeView:** 移除创建会话时对目录选择的强制要求 ([fab80a3](https://github.com/NateScarlet/image-funnel/commit/fab80a3fbe159674deb2468f6abbb364de74610f))
* index out of range panic in session undo ([10b65a2](https://github.com/NateScarlet/image-funnel/commit/10b65a204bf4d3c36bfb8b9197fac508dd2040c1))
* panic index out of range in session undo by tracking index in undo stack ([16c0ae0](https://github.com/NateScarlet/image-funnel/commit/16c0ae0c2396f921051d1a0d6f638efa6627922a))
* Session 只应提交已标记的图片 ([d62f883](https://github.com/NateScarlet/image-funnel/commit/d62f883dc78a2bb86739c0e7b10a486c4ad3bf94))
* **session:** incorrect undo implementation ([d2b0dd7](https://github.com/NateScarlet/image-funnel/commit/d2b0dd72a38c02d202fa50a9413289bf51b307d0))
* SessionView not work ([3610dfd](https://github.com/NateScarlet/image-funnel/commit/3610dfdc14f346c821b7f31f3e69b9687f84be6e))
* **SessionView:** 修复撤销按钮禁用状态逻辑 ([13789fa](https://github.com/NateScarlet/image-funnel/commit/13789fa96e317e5ae98ee4556e355b87c422347a))
* **SessionView:** 修复滑动方向与操作不匹配的问题 ([c0682ee](https://github.com/NateScarlet/image-funnel/commit/c0682ee8a8f32238dc6c3a30c9af4712424ff72c))
* **SessionView:** 修复触摸事件在交互元素上意外阻止默认行为的问题 ([bdc3319](https://github.com/NateScarlet/image-funnel/commit/bdc33190ed3e96e36faf0f960ae714f7b3db82e6))
* **SessionView:** 修复触摸事件处理中的滚动冲突 ([a70b05c](https://github.com/NateScarlet/image-funnel/commit/a70b05c1c5a60728746b98dee6378bd7c688bc0e))
* **session:** 无法跨轮撤销 ([d6bc9b7](https://github.com/NateScarlet/image-funnel/commit/d6bc9b79c53adcd2dc1abe50c0f828735a49b6ef))
* **session:** 添加互斥锁保护Session结构体的并发访问 ([ab53fe8](https://github.com/NateScarlet/image-funnel/commit/ab53fe8557ef4c0580556905b14b46fcf396830c))
* **session:** 跨轮撤销不起作用 ([9b54b75](https://github.com/NateScarlet/image-funnel/commit/9b54b75936a4571493ea5dd41272a0754e81e4c0))
* should not pass unwrapped reactive value ([23275cc](https://github.com/NateScarlet/image-funnel/commit/23275cc61ee1d27bc3c7e8a999d612f116f6f6a9))
* unexpected change in session view ([f90403f](https://github.com/NateScarlet/image-funnel/commit/f90403fa3b60fd7840a9b9bfa601e74bf6d2f0c3))
* UpdateSessionModal 在移动端横屏显示不全无法提交 ([0acc5bc](https://github.com/NateScarlet/image-funnel/commit/0acc5bc468060ab930a86acdf0954b59f53878c8))
* **url签名:** 使用文件修改时间替代当前时间作为时间戳 ([f8f57ab](https://github.com/NateScarlet/image-funnel/commit/f8f57abb0214e3044a582e668a4e31a84702a2cc))
* useQuery stop query unexpectedly ([53069c6](https://github.com/NateScarlet/image-funnel/commit/53069c6fb864ecfc104f86344214117341859ee3))
* **web:** correct operation count ([2c17298](https://github.com/NateScarlet/image-funnel/commit/2c17298e8d2539fe8fcf879271f8a4165864e3cb))
* wrong props usage ([f7b98f3](https://github.com/NateScarlet/image-funnel/commit/f7b98f37fa58ff8c2a40228737b7d08a928840bf))
* **xmpsidecar:** missing namespace on existing file ([28bac18](https://github.com/NateScarlet/image-funnel/commit/28bac18d3078088a1c3d9ab454b2f592e21fe6e7))
* 不应返回ErrSessionNotFound ([bb3c2f8](https://github.com/NateScarlet/image-funnel/commit/bb3c2f8ca9508202a948047ee032f5914c1aa0e6))
* 优化提交确认弹窗的交互逻辑和按钮显示状态 ([b1fed7f](https://github.com/NateScarlet/image-funnel/commit/b1fed7fb79ef21643d6f1de53922846de395b5e7))
* **会话:** 为NewSession函数添加id参数 ([203539a](https://github.com/NateScarlet/image-funnel/commit/203539a7bf9770511fde56a0cb2248876388eff2))
* 会话永远不结束 ([c97bfb9](https://github.com/NateScarlet/image-funnel/commit/c97bfb94c5890967adec5e828f9dd435a495a38f))
* 会话表单缺少默认值 ([157fac7](https://github.com/NateScarlet/image-funnel/commit/157fac75294b875f785b265de126d7f509d05959))
* 使用 pointer-events 锁定图片容器阻止原生滚动 ([22cd1ef](https://github.com/NateScarlet/image-funnel/commit/22cd1efa608e302933595f63d696e744aaf9e656))
* 修复 build.ps1 未检查 pnpm 命令退出码导致构建假成功的问题 ([e9467b0](https://github.com/NateScarlet/image-funnel/commit/e9467b0b26e24d0245ea087df89450fcd557c54e))
* 修复 CommitForm 中 RatingSelector 无法修改值的问题 ([55102d7](https://github.com/NateScarlet/image-funnel/commit/55102d72bc5035e35989e7bcbd9e78f071a41ee8))
* 修复 DirectorySelector 中子组件状态同步问题 ([0b37e44](https://github.com/NateScarlet/image-funnel/commit/0b37e441ba9d4a84894a667b2ddf28a854676bc8))
* 修复更改筛选条件后撤销和提交行为异常 ([9d1bee1](https://github.com/NateScarlet/image-funnel/commit/9d1bee1936d21006879ee3edae1896194f79ba75))
* 修复目录筛选逻辑，关闭开关时应隐藏已达标目录 ([f28ca3f](https://github.com/NateScarlet/image-funnel/commit/f28ca3f3add592f325000c1f0f9b598c20d8b395))
* 修复错误的目录ID处理 ([5873bad](https://github.com/NateScarlet/image-funnel/commit/5873badef8154205d6fc0326f55319bec01d229d))
* 修改预设后默认写入操作应该跟着改变 ([c676d5d](https://github.com/NateScarlet/image-funnel/commit/c676d5d4ad9e6f892d297ca85365b97d9e9d9628))
* 允许从根目录创建会话 ([af9a405](https://github.com/NateScarlet/image-funnel/commit/af9a405dc7583dbb399a24b5b3cc00f502b7ce5a))
* 切换会话不起作用 ([b97e38e](https://github.com/NateScarlet/image-funnel/commit/b97e38ea82130cadfe3c194f4eed577c343b9a4b))
* 切换图片时闪烁 ([54659d1](https://github.com/NateScarlet/image-funnel/commit/54659d14ce3724676c48be513301d55d5da1b5cd))
* 创建会话表单应默认选中根目录 ([842f922](https://github.com/NateScarlet/image-funnel/commit/842f92232d6ccc02154e099e8b1697ae640dae4b))
* 前一张图片可能残留 ([ff81ceb](https://github.com/NateScarlet/image-funnel/commit/ff81ceb7fb0f3656e373f3e63596244bfe06513c))
* 加回意外忽略的 graphql 定义文件 ([0d4d48f](https://github.com/NateScarlet/image-funnel/commit/0d4d48f34dcf0b782600f2e24789c0c4975c8f3f))
* 图片路径获取错误 ([4f7565d](https://github.com/NateScarlet/image-funnel/commit/4f7565d428a4ec66ddded8a16d1132e9137a21b3))
* 应忽略文件变更处理中的文件不存在错误 ([74b86cc](https://github.com/NateScarlet/image-funnel/commit/74b86cc41532be5caaf1fe3a343fb129885f9e1b))
* 开发环境应使用开发模式日志配置 ([0e35cda](https://github.com/NateScarlet/image-funnel/commit/0e35cdad07f9acee85d8716693851fc255d9d596))
* 找不到会话时按钮未居中 ([50f08fb](https://github.com/NateScarlet/image-funnel/commit/50f08fbdcbe717bbf58c425ffdf59d5be00fcb7d))
* 提交会话时的交互问题 ([50e0945](https://github.com/NateScarlet/image-funnel/commit/50e094528aa0b301747d3c885347f211852e3c87))
* 点击按钮后应该自动隐藏移动端菜单 ([8a6d431](https://github.com/NateScarlet/image-funnel/commit/8a6d43117b1c8c14c6aca71f0e5cb7ccb44bcc6f))
* 目录排序错误 ([93903b6](https://github.com/NateScarlet/image-funnel/commit/93903b6c588cd44edc34733753c6f36e101d9cdf))
* 确保 XMP 输出包含 x:xmptk 属性 ([94272a8](https://github.com/NateScarlet/image-funnel/commit/94272a84673b293b99d53f3d68619a04fb07a26f))
* 确保 xmp/imagefunnel/MicrosoftPhoto 命名空间仅定义在 rdf:Description 上 ([91b09c1](https://github.com/NateScarlet/image-funnel/commit/91b09c122c6502977db0713ad14ee825b81bea40))
* 移除 SessionActions 组件加载时的文本变化，避免界面抖动 ([72bc170](https://github.com/NateScarlet/image-funnel/commit/72bc17011804cbaf4e8ecd4e9608db6baa95e357))
* 空目录不显示已达标标签 ([565829b](https://github.com/NateScarlet/image-funnel/commit/565829b3e468ec9dd04ad8ba4a576d4f0c328360))
* 跨轮撤销后会话状态不正确 ([ea4cec5](https://github.com/NateScarlet/image-funnel/commit/ea4cec5ccbbd4eda1d0c4b8c32144e22ead118bb))
* 错误的目录筛选结果 ([5220915](https://github.com/NateScarlet/image-funnel/commit/52209153fb87002f5edd93c95536886fca684b9d))
* 限制图片平移区域以避免与滑动命令冲突 ([8167527](https://github.com/NateScarlet/image-funnel/commit/816752760e53802e6398aba603ab89bd548412bf))
* 预设的保留数不起作用 ([7e6eba9](https://github.com/NateScarlet/image-funnel/commit/7e6eba9892e446736b735d62d5dfa96bc487f452))


### Performance Improvements

* **session:** 跳过未修改的评分写入 ([a1a8fbf](https://github.com/NateScarlet/image-funnel/commit/a1a8fbf8235d31d5ed5fd750efca7257019d4a8f))
* 优化会话清理性能 ([a5db3c8](https://github.com/NateScarlet/image-funnel/commit/a5db3c8523546a3052768d32480e4907b64dbbb8))
* 优化图片元数据读取性能，使用 Go 原生库替代 ImageMagick ([6d88b7d](https://github.com/NateScarlet/image-funnel/commit/6d88b7dead7acc026ed50225562534f5eead66dc))
* 减少会话统计次数 ([fc6a3d9](https://github.com/NateScarlet/image-funnel/commit/fc6a3d90fc62897a149fb97bb6544e5b2111d3b7))
* 异步加载目录统计 ([0b07366](https://github.com/NateScarlet/image-funnel/commit/0b07366892bfcf55e8aea6403d59bde8bd5c5ba1))
* 支持中断目录统计 ([fdada81](https://github.com/NateScarlet/image-funnel/commit/fdada810a8e16269de57902d974d5ffc050f4ed4))
