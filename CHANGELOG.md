# Changelog

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
