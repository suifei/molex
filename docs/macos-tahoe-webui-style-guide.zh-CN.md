# macOS 26 Tahoe 风格 WebUI 设计规范

> 版本：1.2
> 日期：2026-08-12
> 适用范围：管理后台、运维控制台、工具型 Web 应用，以及需要明暗双主题的跨项目界面
> 核心参考：[macOS 26 Tahoe: The Ars Technica review](https://arstechnica.com/gadgets/2025/09/macos-26-tahoe-the-ars-technica-review/)

## 0. 文档定位与边界

这是一份基于 macOS 26 Tahoe 与 Liquid Glass 视觉语言整理出的 WebUI 工程规范。它不是 Apple 官方设计规范，也不声称能在浏览器中复刻 Apple 原生的光学材质。

本文的目标是提炼可跨项目复用的设计原则、语义 token、材质层级、控件规则、响应式策略、无障碍要求和验收方法。实现时应称为“受 Tahoe 启发的 Web 设计”或“Tahoe-inspired Web UI”，不要称为“Apple 官方 Liquid Glass Web 实现”。

MoleX 的产品边界保持不变：Relay、Edge、Target 使用浏览器 WebUI 与 CLI，不提供桌面壳，不恢复 Wails 或桌面专属模式。Apple 风格只用于视觉语言和交互细节，不模拟 macOS 菜单栏、Dock、红黄绿窗口按钮，也不把网页伪装成桌面应用截图。

版权边界：本文只做转述、比较和工程化提炼，不转载 Ars Technica 正文，也不把文章图片复制进仓库。附录仅按页面顺序记录素材主题和研究结论，原始图片仍以文章页面为准。

## 快速导航

- [研究范围与方法](#1-研究范围与方法)
- [页面与全部图片清单](#13-页面结构与全部图片清单)
- [一页结论](#2-一页结论)
- [全文 50 个章节覆盖表](#4-全文-50-个章节覆盖表)
- [语义 Token](#7-语义-token)
- [版面与控件规则](#8-web-管理界面的版面规则)
- [CSS 实现边界](#11-css-实现边界)
- [无障碍与性能](#12-无障碍规范)
- [验收清单](#18-验收清单)
- [74 个独立图片素材索引](#附录-a74-个独立图片素材索引)

## 1. 研究范围与方法

### 1.1 页面覆盖规模

对文章异步画廊加载完成后的正文 DOM 进行结构化核对，结果如下。统计于 2026-08-12，页面后续改版可能改变容器数量，但不会影响本文记录的 74 张内容素材：

| 项目 | 数量 | 说明 |
| --- | ---: | --- |
| 正文字符 | 约 126,000 | 包括文章正文、图注和正文内辅助文本，动态页面会有小幅波动 |
| 约计英文词数 | 约 21,000 | 仅用于衡量文章规模，不作为出版统计 |
| 正式评测章节 | 50 | 文章目录中的 H2/H3 内容章节 |
| 正文单图 Figure | 39 | 页面中的 `<figure>` 单图或动图容器 |
| 非 Figure 图片组 | 13 | 1 个文章头图加 12 组多图画廊 |
| 图片展示组 | 52 | 39 个 Figure 与 13 个非 Figure 图片组之和 |
| 原图链接引用 | 75 | 同一张 `tahoe-light.jpeg` 在两个位置重复出现 |
| 独立图片素材 | 74 | 去重后的完整参考素材集 |
| 用户重点截图 | 20 | 从文章中人工选取的明暗模式与核心材质样本 |

### 1.2 分析维度

每个章节和素材按以下维度分析：

1. 信息层级：内容、导航、工具、状态和浮层之间如何分层。
2. 材质策略：透明度、模糊、饱和度、边缘高光与阴影分别承担什么职责。
3. 可读性：文字与图像叠加时，前景色和背景处理是否稳定。
4. 控件几何：圆角、胶囊、图标、开关、滑杆和分段控件的形态规律。
5. 明暗模式：同一层级在浅色与暗色环境中是否保持相同语义。
6. 动效：变化是否表达状态、来源与层级，而不是单纯装饰。
7. 可访问性：降低透明度、减少动态效果、强制颜色和键盘操作的回退方式。
8. Web 边界：哪些效果可以可靠实现，哪些效果依赖 Apple 原生渲染系统。

### 1.3 页面结构与全部图片清单

页面不是“39 个 Figure 就只有 39 张图”。Ars 把多图对照组放在普通 `div.ars-lightbox` 画廊中，只有当前选中项明显显示；如果只统计 `<img>` 或 `<figure>`，会漏掉 35 张对照素材。本文按原图链接统计，并排除响应式缩略图、站点 Logo、头像、广告和加载动画。

页面中 13 个非 Figure 图片组如下，其中 R 编号对应附录 A：

| 页面图片组 | 素材 | 研究用途 |
| --- | --- | --- |
| 文章头图 | R01 | 高细节壁纸与前景可读性基线 |
| 安装流程对照 | R04-R05 | 左对齐与居中流程的扫描效率 |
| 对话框对照 | R06-R07 | 多语言说明与操作区对齐 |
| Content / Chrome 分层 | R11-R13 | Photos、Finder 图标视图与列表视图的材质分配 |
| 跨设备 Liquid Glass | R14-R17 | iOS 通知、macOS 通知、Sound 和 Control Center 的透明度差异 |
| Safari 三代/跨设备对照 | R18-R20 | 强折射上限、桌面克制实现与旧版基线 |
| 可读性失败组 | R21-R23 | 搜索框、Photos 顶栏和 Messages 的文字重影 |
| 降低透明度组 | R24-R25 | Control Center 与 Photos 的实底回退 |
| 透明菜单栏明暗对照 | R26-R27 | 单一前景色无法覆盖复杂背景 |
| 菜单栏补救组 | R28-R30 | 文字阴影、局部背景和全局降低透明度的效果差异 |
| 六种外观对照 | R08、R32-R36 | Light、Dark、Clear Light/Dark、Tint Light/Dark；R08 为重复引用 |
| Spotlight 视图组 | R46-R48 | 应用网格、视图切换和列表模式 |
| 瞬时指标组 | R70-R71 | 音量与亮度反馈的位置、形态和复用方式 |

其余 39 个 Figure 各包含一个主要素材。两者合计 75 次原图引用；去除外观对照组中重复出现的 R08 后，得到附录 A 的 74 张独立内容素材。这个口径用于证明本文覆盖了页面中的隐藏画廊项，而不只是用户提供的 20 张重点截图。

## 2. 一页结论

### 2.1 设计判断

把它理解为：面向频繁操作用户的工具型 WebUI，采用冷银与石墨中性色、系统蓝主操作、克制的半透明工具层和扎实的内容层。重点是安静、清晰、可扫描，而不是把玻璃效果铺满整个页面。

对 MoleX 这类运维控制台，建议设计参数为：

| 参数 | 建议值 | 含义 |
| --- | ---: | --- |
| 设计变化度 | 3/10 | 稳定网格、明确对齐，不做实验性布局 |
| 动效强度 | 3/10 | 只保留反馈、状态切换和浮层来源动画 |
| 信息密度 | 7/10 | 支持高频扫描，但保留足够行高和分组空间 |

### 2.2 最重要的十条规则

1. 玻璃是工具层，不是内容背景。
2. 主内容、表格、长文本和错误说明必须落在接近实底的表面上。
3. 背景越复杂，浮层越不透明；不要依赖浏览器动态采样背景颜色。
4. 任何文字都不能直接压在另一段文字或高细节图片上。
5. 浅色与暗色模式使用同一套语义层级，不通过简单反相生成。
6. 系统蓝只表示主操作、选择和焦点；绿、橙、红只表示真实状态。
7. 圆角必须有层级规则，不能所有容器都变成胶囊。
8. 动效不得延长任务完成时间，减少动态效果时必须退化为即时切换。
9. 降低透明度必须有手动设置和实底回退，不能只依赖媒体查询。
10. 复用 Tahoe 的秩序、材质和反馈，不复制它的可读性问题与强制图标同形化。

## 3. Tahoe 视觉语言的可复用 DNA

### 3.1 内容与控件分层

Tahoe 最稳定的画面并不是“透明度最高”的画面，而是内容和控件边界最清楚的画面。Finder 主内容接近实白或实黑，侧栏带轻微材质感，顶部工具按钮作为独立胶囊悬浮。Control Center 则相反，它本身就是瞬时工具层，因此可以使用更明显的玻璃材质。

Web 转译规则：

- 内容层优先使用实底或 92% 以上等效遮罩。
- 侧栏、顶部栏和工具组可以使用半透明材质。
- Popover、Toast、上下文菜单可以更明显地表现玻璃，但需要足够遮罩。
- 页面背景只提供轻微色温和空间感，不参与信息表达。

### 3.2 深度来自边缘，不来自重阴影

参考图中的“玻璃感”主要由四件事共同产生：半透明底色、背景模糊、1px 亮边、非常克制的外阴影。单独提高模糊值不会得到同样效果，只会让界面发灰。

推荐的层次组合：

- 上边缘高光表达入射光。
- 外边框表达材料轮廓。
- 内部遮罩保证内容可读。
- 低扩散阴影表达浮层离开背景的距离。

### 3.3 自适应不是无限透明

文章展示了 Tahoe 会根据下方内容调整文字、按钮亮度和不透明度，但也展示了这种处理经常失败。浏览器没有稳定、低成本、跨平台的背景采样机制，因此 WebUI 不应尝试逐像素仿制。

更可靠的策略是预先定义三档材质：Clear、Regular、Strong。设计阶段明确每个组件使用哪一档，而不是让组件自动猜测背景是否复杂。

### 3.4 几何语言

Tahoe 使用连续圆角和胶囊工具组建立柔和感，但不同对象仍有区分：窗口最大、面板次之、控件较小，只有分段选择、状态开关和图标工具组使用完整胶囊。

WebUI 不应把每个文本按钮、标签和信息块都做成胶囊。胶囊只适用于：

- 二到四项的分段模式选择。
- 成组的图标工具。
- 短状态和真实筛选条件。
- 开关、滑杆拇指和紧凑浮动操作。

### 3.5 颜色承担语义

系统蓝负责选择、主操作、链接和焦点。绿色表示健康与成功，橙色表示等待或需要注意，红色表示失败、断开或危险。中性色负责结构。不要用绿色替代所有主按钮，也不要让多种强调色同时竞争。

### 3.6 动效表达材料和来源

“Liquid”部分主要表现在开关、滑杆、通知和 Control Center 的弹性反馈。有效动效有三个特征：

- 从触发源附近出现，用户知道浮层来自哪里。
- 有轻微形变或缩放，但不影响点击时机。
- 失焦时停止持续动画，减少视觉干扰和资源消耗。

### 3.7 个性化有边界

文章展示了浅色、暗色、Clear、Tinted、文件夹颜色和 Control Center 自定义。它也指出用户可以组合出非常不协调的结果。跨项目复用时，应允许用户选择主题和密度，但不要开放任意前景色、背景色和透明度组合。

## 4. 全文 50 个章节覆盖表

本节用于证明研究覆盖了整篇评测，而不是只挑选 20 张截图。相关性分为关键、高、中、低。低相关章节仍保留其产品或工程启示。

### 4.1 平台、品牌与安装

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 1 | System requirements and compatibility | 低 | 设计系统需要明确最低浏览器与设备能力，不应让旧设备静默退化到不可用状态。 |
| 2 | Other system requirements | 中 | 需要能力检测和渐进增强。高级材质、WebGPU 或本地 AI 不能成为基本操作前提。 |
| 3 | What is not ready for launch yet? | 低 | 未完成能力不要占据主界面；为延期功能保留真实状态，而不是假入口。 |
| 4 | Options for owners of unsupported Macs | 低 | 兼容性说明必须给出下一步行动和支持期限。 |
| 5 | Branding: What is in a number? | 低 | 跨产品统一版本命名能降低认知成本，但迁移期需要兼容旧接口和脚本。 |
| 6 | What is in a name? | 低 | 水、透明和流动是视觉隐喻，不应牺牲边界识别。文章对“边界变模糊”的双关正是警告。 |
| 7 | Installer and installation | 高 | 对话框改为左对齐；安装图标进入统一圆角体系；新功能引导应短且可跳过。 |
| 8 | Free space | 中 | 视觉资源、字体和动效同样需要体积预算，精品不等于更重。 |
| 9 | FileVault by default | 中 | 安全默认值应开启，同时保留明确、可理解的退出路径和恢复方案。 |

### 4.2 Liquid Glass 核心

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 10 | Liquid Glass | 关键 | 玻璃是带边缘和动态反馈的半透明材料，不是简单的 `blur()`。动效不能延长等待。 |
| 11 | Consistently inconsistent | 关键 | Clear 与 Strong 材质必须按内容复杂度分配。文字压图、文字压文字和半透明灰字是硬性禁区。 |
| 12 | Reduce transparency | 关键 | 提供一键实底回退，但设计本身仍需可读；不能把无障碍开关当作默认设计缺陷的补丁。 |
| 13 | Invisible menu bar | 高 | 透明导航只有在背景受控时才成立。复杂背景需要遮罩、实底或高对比前景。 |
| 14 | Expansive Control Center | 关键 | 高频控制集中管理，允许适度自定义；图标状态必须真实反映开关状态，避免工具区过载。 |
| 15 | Infinite color options | 高 | 明暗、清透、着色可拆分为有限预设。不要让用户创建不可读的任意配色。 |

### 4.3 图标、搜索与自动化

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 16 | New icons | 高 | 图标需要统一网格、光照和明暗变体，但不能为了同形而抹去识别度。 |
| 17 | When is an icon not an icon? | 高 | 分层图标可自动生成主题变体，但需预生成和缓存，避免列表首次出现时逐个跳出。 |
| 18 | Spotlight | 关键 | 命令面板应键盘优先，搜索、应用、文件、动作和历史要有清晰模式，不把所有结果混成一列。 |
| 19 | Shortcuts and clipboard history | 高 | 快捷动作可自定义短键；敏感历史默认关闭、可清除、保留期明确。 |
| 20 | Automated Shortcuts | 高 | 自动化需要触发条件、是否确认、是否通知和最近运行结果，不允许“静默失败”。 |

### 4.4 Safari 与 Web 能力

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 21 | Safari 26 | 中 | 同一产品在旧平台可保留旧外观，功能兼容比强制视觉一致更重要。 |
| 22 | WebGPU support | 低 | 高级图形能力必须检测后启用，不能作为普通管理界面的必要依赖。 |
| 23 | HDR image support | 中 | HDR 与 SDR 混排需要亮度约束，避免单张素材破坏整体层级。 |
| 24 | SVG favicons | 中 | 矢量图标应支持缩放及明暗模式，减少多尺寸位图维护。 |
| 25 | Pretty text | 高 | 长文本使用 `text-wrap: pretty` 可改善孤行与参差，但只用于有限正文，避免大段内容的性能成本。 |
| 26 | Additional anonymizing | 中 | UI 设计不能依赖不必要的设备指纹信息；隐私保护属于产品质量的一部分。 |

### 4.5 系统应用与生产力

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 27 | Phone | 中 | 宽屏采用主列表加详情面板；键盘输入必须与指针操作同等完整。 |
| 28 | Journal | 低 | 多集合内容需要明确分组、地图或其他真实视图，不应堆叠相同卡片。 |
| 29 | Games | 中 | 内容中心可整合多来源项目，但筛选必须说明来源、安装状态和兼容性。 |
| 30 | Messages | 高 | 右侧详情抽屉比阻塞式弹窗更适合持续上下文；动态背景必须为文字提供稳定背板。 |
| 31 | Notes | 中 | 导入导出需要说明格式损失，内容兼容优先于视觉装饰。 |
| 32 | Terminal | 高 | Clear 主题只加轻微透明；等宽字体、字形覆盖和合理默认尺寸比玻璃更重要。 |
| 33 | Passwords and passkeys | 中 | 高风险迁移使用直接、安全、可撤销的流程，避免明文中间文件和含糊确认。 |
| 34 | Metal 4 | 低 | 性能增强需要在目标硬件上测量，不把理论能力当成体验提升。 |
| 35 | Game Porting Toolkit 3.0 | 低 | 兼容层的成功状态需要区分“可运行”“可用”和“原生体验”。 |

### 4.6 系统反馈、恢复与结论

| # | 章节 | 相关性 | 可复用结论 |
| ---: | --- | --- | --- |
| 36 | Grab Bag | 低 | 小功能也需要进入统一设计系统，不应成为样式例外。 |
| 37 | Lock screen typeface options | 中 | 个性化选择应少而精，并放在用户能找到的设置位置。 |
| 38 | Live translation | 中 | 双语与实时翻译状态要清楚标识来源、进行中和失败状态。 |
| 39 | Live Activities | 高 | 持续状态适合紧凑、邻近来源的胶囊，但只显示最重要的一到两个指标。 |
| 40 | Wallpaper screensavers | 中 | 背景动画在失焦、低电量和减少动态效果时应停止。 |
| 41 | Two-factor autofill | 中 | 一次性验证码可跨浏览器使用，完成后自动清理，同时保留用户知情。 |
| 42 | Game Overlay | 高 | 上下文工具应贴近当前任务，以覆盖层呈现，不强迫用户离开当前页面。 |
| 43 | Notification summaries | 高 | 机器生成摘要必须显著标识并允许按类别关闭；警告不能替代准确性。 |
| 44 | Volume and brightness indicators | 高 | 瞬时反馈应靠近触发源、短暂出现、不阻塞中心内容。 |
| 45 | Low battery alert | 高 | 紧急状态用形状、数字、颜色和文案共同表达；频率与阈值最好可配置。 |
| 46 | Recovery mode changes | 高 | 恢复工具必须说明正在检查什么、是否有进展、结果是什么、下一步做什么。 |
| 47 | New disk image format | 低 | 底层性能优化应转化为可感知的速度或状态信息，不需要占据主要视觉。 |
| 48 | Quantum-safe encryption | 低 | 安全升级应尽量无感，但在文档和状态页中可验证。 |
| 49 | Video and image post-processing | 低 | 媒体增强应渐进启用，避免影响实时操作和低端设备。 |
| 50 | Tahoe clears the decks | 关键 | Liquid Glass 有一致性价值，但过度透明、重叠和图标同形化会带来可用性回退。 |

## 5. 用户提供的 20 张重点截图

20 张截图覆盖了文章最重要的视觉证据，其中第 17 和第 18 张内容几乎相同，可视为同一“降低透明度 Control Center”样本的重复确认。

| 截图 | 主要内容 | 观察结论 |
| ---: | --- | --- |
| S01 | 浅色桌面、Finder、Control Center 与小组件 | 内容窗口较实，浮动控制更透明；同一画面可同时存在多档材质。 |
| S02 | 暗色 Sound Popover | 暗色浮层需要更高遮罩和更弱高光，蓝色只用于当前值与选中设备。 |
| S03 | 设置页应用开关列表 | 长列表保持实底，开关靠右统一对齐，图标只帮助扫描。 |
| S04 | Photos 顶部工具栏 | 工具胶囊覆盖图片时风险最高，必须根据图片复杂度提升遮罩。 |
| S05 | Finder 图标视图 | 侧栏和内容通过材质差异分区，主内容保持高可读性。 |
| S06 | Finder 列表视图 | 稠密表格不应使用清透玻璃，行高、列对齐和弱分隔优先。 |
| S07 | 暗色通知 | 深色遮罩能稳定白字，但过度透明仍会混入背景细节。 |
| S08 | 浅色通知 | 浅色通知使用更强乳白遮罩，说明同一组件不能简单反相。 |
| S09 | 浅色 Sound Popover | 标题、分组标题、设备名和辅助状态形成四级文字层次。 |
| S10 | Control Center 局部 | 胶囊、圆形按钮和大块媒体控件共享半径逻辑，但并非所有元素同形。 |
| S11 | iOS Safari 强玻璃 | 是高透明、高折射感的上限样本，不适合直接照搬到管理后台。 |
| S12 | Tahoe Safari | 桌面端明显更克制，地址栏与工具按钮更接近实底。 |
| S13 | Sequoia Safari | 与 Tahoe 对比可见变化主要来自圆角、工具分组和轻材质，而非信息架构重做。 |
| S14 | Messages 裸文字压背景 | 白字阴影仍不足以保证复杂背景上的可读性，应增加背板。 |
| S15 | Settings 搜索框重影 | 半透明搜索框压在侧栏文字上，是必须避免的文字压文字反例。 |
| S16 | Photos 顶栏重影 | 灰色元数据在图片和玻璃层之间被冲淡，证明次要文字也要满足对比度。 |
| S17 | 降低透明度后的 Control Center | 实底回退大幅改善边界和文字稳定性。 |
| S18 | 同上 | 重复样本确认实底方案仍能保留几何语言和状态颜色。 |
| S19 | Control Center 编辑器 | 自定义采用左侧类别、中间控件库、右侧实时预览，任务关系直观。 |
| S20 | 暗色桌面综合画面 | 暗色主窗口保持接近实黑，浮层保留透明度，白字层级仍需严格控制。 |

## 6. 材质层级

### 6.1 五级材质模型

下表的数值是推荐的 Web 近似值，不是从 Apple 资源提取的官方参数。

| 层级 | 名称 | 典型对象 | 等效遮罩 | Blur | 规则 |
| --- | --- | --- | ---: | ---: | --- |
| M0 | Canvas | 页面背景 | 100% | 0 | 只提供冷暖与空间基调，不承载文字。 |
| M1 | Content | 表格、表单、正文、详情 | 92%-100% | 0-8px | 可读性优先，默认不用玻璃。 |
| M2 | Chrome | 侧栏、顶栏、工具栏 | 74%-88% | 18-24px | 可显示少量下层色彩，不允许下层文字清晰可辨。 |
| M3 | Floating | Popover、菜单、Toast、悬浮工具组 | 78%-92% | 24-30px | 必须有边框、高光和外阴影；复杂背景使用 Strong 变体。 |
| M4 | Critical | 错误、确认、恢复、低电量 | 94%-100% | 0-12px | 信息优先，不追求清透；用语义色强化边缘或图标。 |

### 6.2 Clear、Regular、Strong 决策

| 变体 | 可用场景 | 禁止场景 |
| --- | --- | --- |
| Clear | 纯色或低细节背景上的图标工具组，且文字很少 | 表格、图片墙、日志、复杂壁纸、长文本 |
| Regular | 普通侧栏、顶栏、菜单和短通知 | 文字可能与下层文字重叠的区域 |
| Strong | 图片上方工具、命令面板、错误浮层、复杂桌面背景 | 无特殊禁用，但仍需检查对比度 |

### 6.3 叠层限制

- 同一视觉路径最多出现两层带模糊的材质。
- 玻璃面板内部的卡片优先用半透明实色，不再重复 `backdrop-filter`。
- 页面主体与页面区段不做漂浮卡片；只让真正的工具、浮层或重复条目成为容器。
- 对话框打开时，背景使用遮罩和轻微降对比，不对整个页面再加重模糊。
- 任何玻璃表面在不支持 `backdrop-filter` 时都必须保持完整层级。

## 7. 语义 Token

### 7.1 颜色 Token

这些值与当前 MoleX 冷银、石墨和系统蓝方向一致，可作为跨项目起点。上线前仍需根据品牌色和实际背景做对比度测试。

| Token | 浅色 | 暗色 | 用途 |
| --- | --- | --- | --- |
| `--canvas` | `#e9edf3` | `#111216` | 页面背景 |
| `--canvas-top` | `#f8f9fb` | `#1b1e24` | 背景顶部明度 |
| `--canvas-bottom` | `#e5eaf1` | `#0f1013` | 背景底部明度 |
| `--surface` | `rgb(255 255 255 / 74%)` | `rgb(34 36 42 / 78%)` | Regular 玻璃 |
| `--surface-raised` | `rgb(248 250 253 / 84%)` | `rgb(48 50 57 / 78%)` | 浮层表面 |
| `--surface-solid` | `#f8f9fb` | `#24262c` | 透明度回退 |
| `--surface-raised-solid` | `#ffffff` | `#303239` | 浮层实底回退 |
| `--surface-quiet` | `rgb(45 55 70 / 5.5%)` | `rgb(255 255 255 / 5.5%)` | 轻分组、静止状态 |
| `--surface-hover` | `rgb(45 55 70 / 9%)` | `rgb(255 255 255 / 9%)` | Hover |
| `--input` | `rgb(255 255 255 / 80%)` | `rgb(12 13 16 / 66%)` | 输入框与日志底色 |
| `--border` | `rgb(52 61 74 / 16%)` | `rgb(255 255 255 / 12%)` | 普通边框 |
| `--border-strong` | `rgb(52 61 74 / 29%)` | `rgb(255 255 255 / 23%)` | 强边界、禁用分隔 |
| `--highlight` | `rgb(255 255 255 / 92%)` | `rgb(255 255 255 / 20%)` | 顶部内高光 |
| `--highlight-soft` | `rgb(255 255 255 / 52%)` | `rgb(255 255 255 / 8%)` | 弱内高光 |
| `--text` | `#1d1d1f` | `#f5f5f7` | 主文字 |
| `--text-soft` | `#3a3a3c` | `#d2d2d7` | 次级文字 |
| `--muted` | `#6b6b70` | `#a1a1a6` | 辅助文字，按实底保证小字号对比度 |
| `--faint` | `#7f7f85` | `#85858b` | 非关键元数据，仅用于稳定实底 |
| `--accent` | `#0071e3` | `#0a84ff` | 选择、链接、焦点、主操作 |
| `--accent-text` | `#0068d0` | `#2997ff` | 小字号蓝色文字，需满足至少 4.5:1 |
| `--accent-hover` | `#0064ca` | `#3698ff` | 主操作 Hover |
| `--accent-soft` | `rgb(0 113 227 / 11%)` | `rgb(10 132 255 / 15%)` | 选择背景 |
| `--success` | `#167a35` | `#4cd964` | 健康、已连接、成功 |
| `--warning` | `#9a5b00` | `#ffb340` | 等待、重连、需注意 |
| `--danger` | `#c9342f` | `#ff6961` | 失败、断开、危险操作 |

### 7.2 前景与背景配对

| 背景 | 首选前景 | 可用辅助前景 | 禁止 |
| --- | --- | --- | --- |
| `--canvas` | `--text` | `--text-soft` | 直接放 `--faint` 长文本 |
| `--surface-solid` | `--text` | `--muted` | 低透明灰字叠图片 |
| `--surface` | `--text` | `--text-soft` | 下层仍可辨认时使用 `--faint` |
| `--accent` 浅色 | `#ffffff` | 无 | 灰字、半透明白字 |
| 暗色高亮蓝 | 深色前景或更深的按钮蓝 | 无 | 未测试时默认白字 |
| 状态浅底 | 对应的深色语义文字 | 主文字 | 仅用颜色、不写状态 |

注意：`#0a84ff` 上的白色小字号文字不应凭肉眼判断。暗色主题的填充按钮可以继续使用较深的 `#0071e3` 配白字，或使用较亮蓝色配深色文字。链接和焦点环不受这条限制。

### 7.3 排版 Token

不要在 Web 项目中打包或重新分发 SF Pro。优先使用系统字体栈：

```css
font-family:
  system-ui,
  -apple-system,
  BlinkMacSystemFont,
  "Segoe UI Variable Text",
  "Segoe UI",
  "Microsoft YaHei UI",
  "PingFang SC",
  sans-serif;
```

技术值另用系统等宽栈，不下载或打包第三方 Web Font：

```css
font-family:
  ui-monospace,
  "SFMono-Regular",
  "Cascadia Mono",
  "Segoe UI Mono",
  Consolas,
  "Liberation Mono",
  monospace;
```

| Token | 字号/行高 | 字重 | 用途 |
| --- | --- | ---: | --- |
| `--type-page` | `24px/32px` | 680-700 | 页面标题 |
| `--type-section` | `18px/26px` | 650-680 | 区段标题 |
| `--type-body` | `14px/20px` | 400 | 正文与表单值 |
| `--type-label` | `13px/18px` | 550-600 | 标签、按钮、表头 |
| `--type-meta` | `12px/17px` | 400-550 | 时间、平台、辅助信息 |
| `--type-code` | `13px/19px` | 400-550 | IP、端点、ID、日志 |

约束：

- 字距保持 `0`，不使用负字距营造“苹果感”。
- 中文和英文必须使用相同语义层级，不能因英文更短而压缩中文行高。
- IP、端口、字节数和时间使用等宽数字或等宽字体，但普通正文不使用等宽字体。
- “未监听”“未暴露”“空闲”等自然语言状态必须使用界面字体；只有真实地址、端口、路由 ID、密钥和日志内容使用等宽字体。
- `text-wrap: pretty` 适合有限长度的说明，不应用于大型日志或虚拟列表。

### 7.4 间距、圆角与尺寸

| 类别 | Token | 建议值 |
| --- | --- | ---: |
| 基础间距 | `--space-1` 到 `--space-8` | `4, 8, 12, 16, 20, 24, 32, 40px` |
| 页面水平边距 | `--page-pad` | 桌面 `24px`，平板 `20px`，手机 `14-16px` |
| 窗口/工作区 | `--radius-window` | `16px` |
| 浮层/大面板 | `--radius-panel` | `12-14px` |
| 普通控件 | `--radius-control` | `8-10px` |
| 紧凑条目 | `--radius-compact` | `6-8px` |
| 胶囊 | `--radius-pill` | `999px`，只用于明确的胶囊对象 |
| 桌面控件高度 | `--control-h` | `36px` |
| 紧凑控件高度 | `--control-h-sm` | `30-32px` |
| 触控目标 | `--touch-target` | 至少 `44px` |

### 7.5 动效 Token

| Token | 值 | 用途 |
| --- | --- | --- |
| `--motion-fast` | `120ms` | Hover、按下、图标状态 |
| `--motion-base` | `180ms` | 选择、开关、短浮层 |
| `--motion-panel` | `240ms` | 抽屉、Popover、详情面板 |
| `--ease-standard` | `cubic-bezier(.2,.8,.2,1)` | 普通状态变化 |
| `--ease-out` | `cubic-bezier(.16,1,.3,1)` | 浮层进入 |
| `--press-scale` | `.98` | 按钮按下反馈 |

### 7.6 主题解析

主题偏好必须有三个值：`system`、`light`、`dark`。`system` 是默认值，解析结果写入根元素的 `data-theme`，组件只消费解析后的语义 token。

```js
const media = window.matchMedia("(prefers-color-scheme: dark)");
const savedPreference = localStorage.getItem("theme") ?? "system";

function resolveTheme(preference) {
  return preference === "system"
    ? (media.matches ? "dark" : "light")
    : preference;
}

document.documentElement.dataset.theme = resolveTheme(savedPreference);
```

- 只有偏好为 `system` 时才响应系统主题变化。
- 在应用脚本加载前用一小段内联启动脚本解析主题，避免浅色和暗色之间闪屏。
- 同步设置 `color-scheme`，让原生表单、滚动条和浏览器控件匹配主题。
- 主题切换不重置业务状态，也不对整页播放大面积颜色过渡。
- 服务端渲染项目应通过 Cookie 或首屏脚本保证客户端与服务端结果一致。

## 8. Web 管理界面的版面规则

### 8.1 桌面布局

工具型 WebUI 推荐采用稳定的三段式结构：

```text
┌──────────────────────── Top bar ────────────────────────┐
│ Brand / runtime status                 global actions  │
├──────────────┬──────────────────────────┬───────────────┤
│ Navigation   │ Main content             │ Inspector     │
│ 220-260px    │ minmax(0, 1fr)           │ 300-360px     │
│ optional     │                          │ optional      │
└──────────────┴──────────────────────────┴───────────────┘
```

- 顶栏只放全局状态、语言、主题、账户和真正的全局命令。
- 侧栏只承载一级导航和稳定筛选，不把所有指标塞入侧栏。
- 主内容是唯一滚动主体，避免多个同级滚动容器争夺滚轮。
- Inspector 只在查看某个客户端、任务或事件详情时出现。
- Inspector 从右侧滑入，保留主列表上下文；需要强确认时才使用模态框。
- 不绘制假的 macOS 窗口边框和交通灯按钮。

### 8.2 路径可视化

路由、转发链路和处理流水线应把图标与文字分成两条稳定轨道：第一条轨道只放节点图标和连接线，第二条轨道只放标签。禁止让不同长度的标签参与图标的垂直居中计算，否则中英文切换、长域名或 IPv6 会把节点推离同一水平线。

- 所有节点图标中心和连接线中心必须处于同一个纵坐标，验收误差不超过 `1px`。
- 入口、处理中枢、出口按实际处理顺序从左到右排列；单向转发在线尾使用明确箭头，双向链路才使用双向箭头。
- 箭头与线条共享状态色：空闲为中性灰，连接或重连为警告色，稳定运行才使用成功色。
- 标签在各自节点下方居中，长值单行省略并通过详情或提示展示完整内容，不得改变图标轨道高度。
- 移动端可以缩小两侧图标，但图标轨道高度必须由最大的中枢图标决定；连接线仍按轨道中心定位。
- 动画只能沿连接线表达“正在建立路径”，不能通过上下浮动图标制造位移。

### 8.3 列表与详情

已连接客户端、转发规则和事件日志属于高密度信息，不适合用大量独立卡片。推荐：

- 桌面宽屏使用表格或结构化列表。
- 首列放名称和角色，第二列放连接 IP，第三列放转发端点，后续列放状态、连接时长和流量。
- IP、端口、路由 ID 和字节数使用等宽数字。
- 默认显示人能识别的信息，完整 ID 放入详情或复制操作。
- 一行只使用一个真实状态标记，不在每个字段前加装饰圆点。
- 行 Hover 只改变轻背景，不改变行高、边框厚度或文本位置。
- 选中行使用 `--accent-soft` 加左侧或内侧强调，不使用重蓝整行填充。

### 8.4 响应式规则

| 宽度 | 布局 |
| ---: | --- |
| `>= 1180px` | 顶栏、侧栏、主内容，按需显示 Inspector |
| `820-1179px` | 侧栏压缩或变为抽屉，主内容保留两列配置布局 |
| `390-819px` | 单列主内容，表格改为字段分组行，工具栏允许分成两行 |
| `320-389px` | 单列、紧凑边距、图标按钮优先，长端点强制换行或中间省略 |

每个断点都必须检查：

- 中英文切换后按钮不截断。
- IP、域名、IPv6 和错误信息不产生水平滚动。
- 固定工具栏不遮挡首行或末行内容。
- 触控目标至少 44px，视觉图标可以保持 20px。
- 表格变形后仍保留字段标签，不能只剩一串无上下文的值。

## 9. 控件规范

### 9.1 按钮

| 类型 | 视觉 | 使用规则 |
| --- | --- | --- |
| Primary | 系统蓝实底 | 每个任务区最多一个，表示启动、保存、连接等主命令 |
| Secondary | Raised 或浅实底 | 普通命令，不与 Primary 争夺视觉焦点 |
| Ghost | 无底或弱 Hover 底 | 顶栏工具和低频命令，必须有稳定 Hover 与 Focus |
| Destructive | 红色浅底或红色实底 | 删除、断开、清空等不可逆或高风险操作 |
| Icon-only | 正方形或圆形 | 使用熟悉图标，必须有 `aria-label` 和 Tooltip |

规则：

- 文本命令保持一行，桌面端不允许按钮文案换行。
- 图标与文字组合时，图标 16-18px，间距 6-8px。
- `:active` 使用 `scale(.98)`，不改变尺寸和布局。
- Disabled 不能只降低透明度到看不清，仍需保留文字可读性。
- 同一意图只使用一个稳定文案，例如全站统一使用“保存”而不是混用“应用”“提交”“确认保存”。

### 9.2 输入框与搜索

- 标签放在输入框上方，不使用 Placeholder 代替标签。
- 默认高度 36px，错误信息放在输入框下方。
- 搜索框可使用胶囊外形，但必须有足够实底，避免出现参考图中的文字重影。
- 聚焦使用系统蓝 3px 外环或双层环，不仅改变边框颜色。
- 密钥、Token 和密码默认遮蔽，页面和日志中不得回显。
- 长端点允许选择、复制和完整查看，不能只显示不可访问的省略文本。

### 9.3 开关

- 开关只用于立即生效的二元设置。
- 需要保存、存在依赖或会中断连接的设置，不应伪装成即时开关。
- 桌面推荐轨道约 36x20px，拇指 16px。
- On 使用蓝色，Off 使用中性灰；成功状态不等于开关 On，因此不要用绿色。
- 标签点击区域也应切换开关，并保持可见焦点。
- 加载或等待后端确认时，锁定重复操作并显示短状态，不让拇指反复跳动。

### 9.4 分段控件

- 只用于 2-4 个互斥、同层级且频繁切换的选项。
- 当前项使用接近实底的浮起块，其他项保持透明或弱底。
- 不把导航、筛选和操作混在同一个分段控件中。
- 中文文案较长时允许控件加宽，不缩小字号。

### 9.5 滑杆

- 滑杆只用于连续数值，并同时显示当前值或可访问名称。
- 轨道与拇指要在明暗模式中保持至少 3:1 的非文本对比。
- 拖动时可以增加拇指高光和轻微缩放，但不要动画模糊半径。
- 键盘方向键、Home、End 和 Page Up/Down 应可操作。

### 9.6 状态标签

MoleX 推荐状态映射：

| 状态 | 颜色 | 推荐文案示例 |
| --- | --- | --- |
| Healthy / Connected | 绿色 | 已连接 / Connected |
| Connecting / Reconnecting / Waiting | 橙色 | 正在重连 / Reconnecting |
| Idle / Stopped | 中性灰 | 已停止 / Stopped |
| Failed / Disconnected | 红色 | 连接失败 / Connection failed |
| Selected / Active mode | 蓝色 | 当前模式 / Active mode |

状态不能只靠颜色表达。图标、文本和必要的下一步操作必须同时存在。

### 9.7 错误、警告和恢复状态

一条可操作错误至少包含四部分：

1. 发生了什么。
2. 最可能的原因。
3. 用户现在应该检查什么。
4. 系统会自动重试，还是需要用户操作。

推荐结构：

```text
无法连接到中继
127.0.0.1:8080 拒绝了连接。请确认 Relay 已启动，并检查 Caddy 上游地址。
将在 8 秒后重试。
[立即重试] [查看诊断]
```

不要只显示“Something went wrong”“连接异常”或未经解释的错误码。恢复工具同样需要显示当前步骤和结果，避免文章所批评的“只说正在检查，但不说明检查什么”。

### 9.8 Popover、Toast 与通知

- Popover 尽量贴近触发按钮，并在触发按钮方向上对齐。
- Toast 放在不会遮挡主任务的位置，默认右上或右下，但移动端使用底部安全区。
- 短反馈 3-5 秒后消失；错误和需要操作的通知不自动消失。
- 通知标题不超过一行，正文不超过三行，复杂内容进入详情页。
- 紧急状态使用 Strong 或 Critical 材质，不使用最透明的 Clear。
- 瞬时指标从来源附近出现，例如音量、流量限速或连接质量变化。

### 9.9 命令面板与搜索

从 Spotlight 可复用的模式：

- 键盘优先，打开后焦点直接进入搜索框。
- 将“搜索对象”和“执行动作”分开，避免误触。
- 最近使用、应用/对象、动作和剪贴板历史使用清晰的模式切换。
- 敏感历史默认关闭，提供保留期、清除和全局关闭。
- 快捷键显示为辅助信息，不占据主标题位置。
- 搜索结果必须有类型、来源和状态，不只显示名称。

## 10. 动效与时序

### 10.1 可用动效

| 场景 | 进入 | 退出 | 说明 |
| --- | ---: | ---: | --- |
| Hover / Press | 120ms | 100ms | 只动画颜色、Transform、Opacity |
| Switch / Segment | 160-180ms | 140-160ms | 拇指或选择块平滑移动 |
| Popover / Menu | 180-220ms | 140-180ms | 从触发源缩放和淡入 |
| Side inspector | 220-260ms | 180-220ms | 水平移动不超过自身宽度 |
| Toast | 180-220ms | 160-200ms | 轻微位移 8-12px |
| Route state | 180ms | 180ms | 颜色和图标变化，不做无限脉冲 |

### 10.2 时序规则

- 启动、停止、连接和重连的视觉状态必须服从真实运行时状态，不能提前显示成功。
- 同一操作的按钮状态、状态标签、事件流和 Toast 必须使用同一个状态来源。
- 连续 SSE 更新只更新内容，不反复播放整个组件的进入动画。
- 重连倒计时每秒更新时不得改变容器宽度。
- 动画结束不是业务状态完成的条件。
- 所有自动动画在 `prefers-reduced-motion: reduce` 下关闭或变为即时状态切换。

## 11. CSS 实现边界

### 11.1 可维护的 Web 近似实现

```css
:root {
  color-scheme: light;
  --surface: rgb(255 255 255 / 74%);
  --surface-solid: #f8f9fb;
  --border: rgb(52 61 74 / 16%);
  --highlight: rgb(255 255 255 / 92%);
  --text: #1d1d1f;
  --shadow-float:
    0 18px 48px rgb(49 62 79 / 16%),
    0 2px 8px rgb(49 62 79 / 8%);
}

:root[data-theme="dark"] {
  color-scheme: dark;
  --surface: rgb(34 36 42 / 78%);
  --surface-solid: #24262c;
  --border: rgb(255 255 255 / 12%);
  --highlight: rgb(255 255 255 / 20%);
  --text: #f5f5f7;
  --shadow-float:
    0 20px 56px rgb(0 0 0 / 38%),
    0 2px 8px rgb(0 0 0 / 20%);
}

.glass-surface {
  position: relative;
  isolation: isolate;
  overflow: clip;
  color: var(--text);
  border: 1px solid var(--border);
  background:
    linear-gradient(180deg, var(--highlight), transparent 42%),
    var(--surface);
  -webkit-backdrop-filter: blur(24px) saturate(160%);
  backdrop-filter: blur(24px) saturate(160%);
  box-shadow: inset 0 1px 0 var(--highlight), var(--shadow-float);
}

@supports not ((-webkit-backdrop-filter: blur(1px)) or
               (backdrop-filter: blur(1px))) {
  .glass-surface {
    background: var(--surface-solid);
  }
}

@media (prefers-reduced-transparency: reduce) {
  .glass-surface {
    background: var(--surface-solid);
    -webkit-backdrop-filter: none;
    backdrop-filter: none;
  }
}

:root[data-transparency="reduce"] .glass-surface {
  background: var(--surface-solid);
  -webkit-backdrop-filter: none;
  backdrop-filter: none;
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    scroll-behavior: auto !important;
    animation-duration: .001ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: .001ms !important;
  }
}

@media (forced-colors: active) {
  .glass-surface {
    border: 1px solid CanvasText;
    color: CanvasText;
    background: Canvas;
    box-shadow: none;
  }
}
```

### 11.2 可以做

- 使用语义 CSS 变量管理明暗主题。
- 使用 `backdrop-filter: blur() saturate()` 做渐进增强。
- 使用 1px 边框、内高光和低扩散阴影表达材料边缘。
- 只动画 `transform` 和 `opacity`，颜色变化保持短促。
- 通过 `@supports`、媒体查询和手动设置提供实底回退。
- 对固定尺寸工具、棋盘、图标组和表格列设置稳定约束，避免动态内容导致布局跳动。

### 11.3 不应做

- 不使用 SVG displacement、Canvas 像素采样或复杂 Blend Mode 假装原生折射。
- 不动画 `backdrop-filter`、Blur 半径、宽高和大面积阴影。
- 不在滚动容器上叠加大面积动态噪点或滤镜。
- 不使用三层以上嵌套玻璃。
- 不把整页内容降低透明度后再靠阴影补救。
- 不复制 Apple 图标、壁纸或 SF 字体文件进入项目。
- 不用 CSS 绘制假的 macOS 窗口和桌面环境。

## 12. 无障碍规范

### 12.1 对比度

- 普通文字至少达到 WCAG AA 4.5:1。
- 18px 以上大文字和非文本控件至少达到 3:1。
- 主正文以 AAA 7:1 为目标。
- 玻璃表面的对比测试必须覆盖最亮、最暗和最高细节三类背景。
- 无法证明所有背景都可读时，提升遮罩到 Strong，而不是继续增加文字阴影。
- Placeholder、禁用态、辅助说明和错误文本同样需要测试。

### 12.2 透明度

`prefers-reduced-transparency` 的浏览器支持并不完整，因此必须同时提供应用内“降低透明度”设置。设置应写入用户偏好，并在登录页、主界面、Popover 和通知上统一生效。

降低透明度后应发生以下变化：

- 所有 Glass 表面切换到 `--surface-solid` 或 `--surface-raised-solid`。
- 关闭 `backdrop-filter`。
- 保留圆角、边框、层级和状态色。
- 不隐藏信息，也不改变操作顺序。

### 12.3 动效、输入与语义

- 尊重 `prefers-reduced-motion`，取消弹性、位移和持续动画。
- 全部功能可通过键盘完成，焦点顺序与视觉顺序一致。
- Icon-only 按钮提供可访问名称和 Tooltip。
- 状态变化通过 ARIA Live 区域谨慎播报，不连续朗读高频流量数据。
- 表格使用真实表头；响应式卡片化后仍保留字段名称。
- 文本缩放到 200% 时不遮挡、不截断、不丢失操作。
- 支持 `forced-colors`，不要在强制颜色模式下依赖背景图片或透明度。
- 触摸场景使用至少 44x44px 命中区域。

## 13. 性能预算

### 13.1 玻璃预算

- 同屏大面积 Blur 表面不超过 2 个。
- 同屏小型 Blur 浮层建议不超过 4 个。
- 只对固定或低频移动的区域使用 `backdrop-filter`。
- 列表每一行、每一张卡片和每个状态标签都禁止单独 Blur。
- 在低性能设备上通过能力检测或设置自动切换到实底。

### 13.2 动效与资源

- LCP 目标小于 2.5 秒，INP 小于 200ms，CLS 小于 0.1。
- 图标主题变体预生成或缓存，避免出现文章所记录的逐个 Icon Pop-in。
- 大图必须声明宽高或 `aspect-ratio`。
- 持续背景动画在页面失焦、减少动态效果和节能模式下停止。
- WebGPU、HDR 或高级滤镜只做可选增强，不能阻塞基本管理功能。

## 14. 中英文与本地化

- 所有可见字符串同时提供 `zh-CN` 与 `en`。
- 布局按中文长度设计，英文只是较短的一个测试样本。
- 按钮优先使用短动词，例如“重试 / Retry”“保存 / Save”“停止 / Stop”。
- 错误说明可以换行，不通过缩小字号塞进一行。
- 日期、时间、数值和字节单位使用语言环境格式化。
- IP、域名、端口、命令和配置键保持原样，不进行翻译。
- 中英文切换不得重置表单、筛选、滚动位置或运行时状态。
- Tooltip、空状态、错误原因和下一步操作也必须本地化。

## 15. 从文章中提炼出的反例

以下问题来自文章对 Tahoe 的直接观察，跨项目复用时应明确禁止：

1. 文字压在另一段文字上，只靠 Blur 和阴影维持可读性。
2. 灰色元数据直接压在高细节图片上。
3. 同类组件在不同页面随意切换 Clear 与 Strong，缺少规则。
4. 透明菜单或导航只靠极弱阴影适应复杂背景。
5. 把“降低透明度”当成修复默认设计的唯一办法。
6. 所有图标强制同一圆角方形，导致识别度和个性下降。
7. 用户可任意组合颜色，最终产生冲突和低对比配色。
8. 菜单栏或工具栏允许无限添加项目，挤压真正的导航空间。
9. 图标主题化在运行时生成，导致窗口打开后逐个出现。
10. 动态聊天背景持续运动，降低时间、名称和状态文字的可读性。
11. 状态按钮外观不随真实状态变化，用户无法一眼判断开关结果。
12. 低电量或其他警告频率固定且无法配置，形成通知疲劳。
13. 恢复工具不说明检查步骤、进度、发现和下一步。
14. 为了统一视觉而改变熟悉控件的位置，却没有明显体验收益。

## 16. 跨项目采用流程

### 16.1 第一步：判断产品类型

| 产品 | 玻璃强度 | 信息密度 | 建议 |
| --- | --- | --- | --- |
| 运维/管理后台 | 低 | 高 | 只用于 Chrome 和 Floating，内容层实底 |
| 消费型工具 | 中 | 中 | 可增加工具胶囊和轻弹性动效 |
| 媒体浏览 | 中到高 | 低到中 | 图片上方必须使用 Strong 工具层 |
| 营销展示 | 中 | 低 | 可强调材质，但仍需实图和内容可读性 |
| 无障碍关键服务 | 极低 | 中 | 默认实底，只保留几何、色彩和层级 |

### 16.2 第二步：只定义三种表面

先实现 Content、Chrome、Floating 三类表面。只有确有需要时再增加 Critical。不要为每个组件创建独立玻璃配方。

### 16.3 第三步：先做实底，再加渐进增强

在关闭 `backdrop-filter` 的状态下完成整个界面和对比度验收。随后加入透明度和 Blur。如果实底版本不成立，玻璃版本也不会成立。

### 16.4 第四步：建立真实状态矩阵

至少覆盖 Loading、Empty、Connecting、Healthy、Reconnecting、Stopping、Stopped、Error 和 Permission denied。每个状态定义文案、颜色、图标、可用操作和自动恢复行为。

### 16.5 第五步：双主题和双语言一起开发

不要在浅色中文版完成后再补暗色和英文。组件第一次实现时就同时检查四种组合：浅色中文、浅色英文、暗色中文、暗色英文。

## 17. MoleX 采用概要

MoleX 可直接采用本文 token，但材质分配应保持克制：

- 登录页：顶部栏 M2，登录面板 M2/M3，背景不放装饰性大段文案。
- 主工作区：整体 M1，顶栏 M2，配置与运行区通过分隔和空间组织，不套多层卡片。
- Relay 已连接客户端：主列表 M1，行详情 Inspector M2，复制和更多操作 Popover M3。
- Edge/Target 状态：主状态使用文字、图标和语义色；重连倒计时宽度固定。
- 日志和事件：实底、等宽、可筛选，不使用 Glass 行。
- 错误：M4，说明原因、重试计划和下一步检查项。
- IP、节点名、角色、转发端点、对端、平台、连接时间、最近活动和加密流量都应可扫描显示。
- 管理界面仍然只通过浏览器提供，不增加桌面窗口装饰或桌面模式。

## 18. 验收清单

### 18.1 视觉

- [ ] 玻璃只用于 Chrome、Floating 或确有层级意义的表面。
- [ ] 页面区段没有被包装成一层层漂浮卡片。
- [ ] 同类组件使用同一档 Clear、Regular 或 Strong。
- [ ] 圆角遵循窗口、面板、控件、紧凑条目的层级。
- [ ] 系统蓝只用于主操作、选择和焦点。
- [ ] 绿色、橙色、红色只表示真实语义状态。
- [ ] 浅色和暗色都没有前景与背景混在一起的区域。
- [ ] 没有文字压文字、灰字压复杂图片或透明按钮消失的问题。

### 18.2 行为

- [ ] Hover、Focus、Active、Disabled、Loading、Success、Error 都存在。
- [ ] 动画不改变布局尺寸，不延长业务操作。
- [ ] SSE 或高频更新不会反复触发进入动画。
- [ ] 重连、停止和空闲状态不会把配置地址误报为正在监听。
- [ ] Popover、Toast 和 Inspector 都能通过 Escape 关闭并恢复焦点。
- [ ] 破坏性操作有适当确认，普通操作不滥用模态框。

### 18.3 无障碍

- [ ] 正文对比度至少 4.5:1，控件边界至少 3:1。
- [ ] 复杂背景上使用 Strong 或实底，而不是单纯文字阴影。
- [ ] 应用内可手动降低透明度。
- [ ] `prefers-reduced-motion`、`prefers-reduced-transparency` 和 `forced-colors` 有回退。
- [ ] 键盘可完成所有功能，焦点清晰可见。
- [ ] 200% 文本缩放无截断和重叠。
- [ ] 状态不只靠颜色表达。

### 18.4 响应式与本地化

- [ ] 已检查 320、390、820、1366px 宽度。
- [ ] 中文和英文均无按钮换行、文字裁切或横向溢出。
- [ ] IPv6、长域名、端点和错误信息有明确换行或省略策略。
- [ ] 移动端表格转换后仍显示字段标签。
- [ ] 固定顶栏、底部安全区和弹出层不遮挡内容。

### 18.5 性能

- [ ] 同屏大面积 Blur 不超过 2 个。
- [ ] 列表行和重复卡片不单独使用 `backdrop-filter`。
- [ ] 不支持 Blur 或低性能时，实底版本仍完整可用。
- [ ] 动态背景在失焦、节能和减少动态效果时停止。
- [ ] 图标、图片和字体不会造成明显布局跳动。

## 附录 A：74 个独立图片素材索引

索引按文章出现顺序编号。它记录“看什么”和“能复用什么”，不复制原图。文件名仅用于在原文中定位。

### A.1 背景、硬件与安装流程

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R01 | `macos-tahoe-default-wallpaper.jpg` | 文章头图与 Tahoe 默认壁纸，蓝色水域和高细节自然背景 | 高细节背景只能作为氛围层；其上的导航和文字必须使用稳定遮罩。 |
| R02 | `Apple_new-macbook-air-wallpaper-screen_03182020.jpg` | 2020 MacBook Air 产品图 | 主要用于兼容性叙事，不是界面样式依据。 |
| R03 | `Installer-icons.jpg` | 近十年安装器图标对比，新图标首次进入圆角方形体系 | 统一图标网格能提升一致性，但历史识别符号不应无理由消失。 |
| R04 | `tahoe-setup-left-align.jpeg` | Tahoe 安装流程左对齐文案 | 表单和说明采用左对齐，更适合扫描、多语言和长文本。 |
| R05 | `sequoia-setup-center-align.jpeg` | Sequoia 安装流程居中文案 | 只适合短、单一、仪式感强的步骤，不适合复杂配置。 |
| R06 | `tahoe-dialog-left.jpeg` | Tahoe 对话框左对齐 | 标题、说明和操作形成清晰阅读起点；按钮区仍需稳定对齐。 |
| R07 | `sequoia-dialog-center.jpeg` | Sequoia 对话框居中 | 对比样本说明视觉对称不一定等于操作效率。 |

### A.2 Liquid Glass、可读性与 Control Center

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R08 | `tahoe-light.jpeg` | 浅色 Tahoe 综合界面，Finder、桌面、Dock 与控制中心同屏 | 同一场景使用多档材质。窗口内容较实，浮层更透明，状态色有限。 |
| R09 | `liquid-glass-audio.webp` | 音量滑杆拖动时的玻璃变化 | 拖动反馈可增加高光和轻缩放，但不改变任务完成时机。 |
| R10 | `liquid-glass-toggle.webp` | 设置开关的弹性动画 | 弹性只服务状态确认；等待后端时需要独立的 Pending 状态。 |
| R11 | `photos-liquid-glass.jpeg` | Photos 图片内容延伸到玻璃顶栏下方 | 媒体应用可以使用 Clear Chrome，但按钮与文字需要 Strong 背板策略。 |
| R12 | `liquid-glass-finder-2.jpeg` | Finder 图标视图与半透明侧栏、顶部工具组 | 内容实底、侧栏 Regular、工具胶囊 Strong，体现清晰层级。 |
| R13 | `liquid-glass-finder.jpeg` | Finder 列表视图 | 稠密列表保持实底；表头、行、列和滚动稳定性优先于材质。 |
| R14 | `ios-26-notification.jpeg` | iOS 通知使用更清透、受背景影响更大的玻璃 | 属于透明上限样本，不适合高密度 Web 管理界面。 |
| R15 | `liquid-glass-notification.jpeg` | macOS 通知遮罩更强 | 桌面通知更克制，证明跨设备不应机械使用同一透明度。 |
| R16 | `liquid-glass-sound.jpeg` | Sound Popover，分组、滑杆、设备状态和入口 | 浮层内部仍需明确标题层级、分隔、选中状态和辅助值。 |
| R17 | `liquid-glass-control-center.jpeg` | Control Center 的胶囊、圆形按钮、媒体和滑杆 | 不同控件共享材质与半径逻辑，但尺寸由功能决定，不能全部同形。 |
| R18 | `ios-26-safari.jpeg` | iOS Safari 强折射工具层 | 适合作为视觉灵感上限，不作为浏览器 WebUI 的默认实现目标。 |
| R19 | `tahoe-safari.jpeg` | Tahoe Safari 顶栏与地址栏 | 桌面端使用更实、更清楚的控件分组；内容区不被玻璃侵入。 |
| R20 | `sequoia-safari.jpeg` | Sequoia Safari 对照 | Tahoe 的升级主要来自几何、分组和轻材质，信息架构可保持稳定。 |
| R21 | `tahoe-settings-smear.jpeg` | 搜索框与下层侧栏文字发生重影 | 文字压文字是硬性失败。搜索框必须提升遮罩或改用实底。 |
| R22 | `tahoe-photos-smear.jpeg` | Photos 顶栏文字和按钮被图片细节冲淡 | 图片上方的次要文字也需要背板，不能只保护主标题。 |
| R23 | `tahoe-text-smear.jpeg` | Messages 名称、时间等裸文字压动态背景 | 文字阴影不足以替代背景遮罩；元数据同样要满足对比度。 |
| R24 | `reduce-transparancy-control-center.jpeg` | 降低透明度后的 Control Center | 实底回退保留圆角与功能，显著提高边界和文字稳定性。 |
| R25 | `photos-reduce-transparency.jpeg` | 降低透明度后的 Photos | 内容不再穿过标题栏，证明 Content 与 Chrome 可以清楚分离。 |
| R26 | `invisible-menu-bar.jpeg` | 浅色壁纸上的透明菜单栏 | 透明顶栏只能用于受控背景；Web 页面通常应使用 Regular 或 Strong。 |
| R27 | `invisible-menu-bar-dark.jpeg` | 暗色背景上的透明菜单栏 | 前景可根据背景切换明暗，但复杂图片仍可能同时含亮区和暗区。 |
| R28 | `invisible-menu-bar-drop-shadow.jpeg` | 系统为菜单文字添加极弱阴影 | 阴影只能辅助，不能作为跨背景对比的主要保障。 |
| R29 | `show-menu-bar-background.jpeg` | 用户重新开启菜单栏背景 | 对透明导航提供局部实底选项，比全局关闭所有透明更精确。 |
| R30 | `menu-bar-reduce-transparency.jpeg` | 全局降低透明度后的菜单栏 | 全局回退是必要安全网，但组件仍应支持更细粒度的 Strong 变体。 |
| R31 | `control-center-customize.jpeg` | 左侧分类、中间控件库、右侧实时 Control Center 预览 | 自定义编辑器应把资源、目标和结果同时呈现，并限制无意义的过度配置。 |

### A.3 明暗外观与图标系统

R08 在外观对比组中重复出现，因此去重后不再次编号。

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R32 | `tahoe-dark.jpeg` | 暗色 Tahoe 综合界面 | 主窗口接近实黑，浮层保留透明度；暗色不是浅色反相，而是重新平衡遮罩和高光。 |
| R33 | `tahoe-clear-light.jpeg` | 浅色 Clear 外观 | Clear 适合低细节背景和低密度工具，不适合长文本或表格。 |
| R34 | `tahoe-clear-dark.jpeg` | 暗色 Clear 外观 | 暗色 Clear 仍需要可见轮廓和足够前景对比，不能只降低 Alpha。 |
| R35 | `tahoe-tint-light.jpeg` | 浅色 Tinted 外观 | 着色应是有限预设，并保持所有语义色可区分。 |
| R36 | `tahoe-tint-dark.jpeg` | 暗色 Tinted 外观 | 同一着色在暗色中需要重新校准亮度和饱和度，不能直接复用浅色值。 |
| R37 | `icon-compare-2.jpg` | 旧图标越界元素被收进圆角方形 | 统一边界会损失个性，Web 项目只统一画布和光照，不强制所有品牌标识同形。 |
| R38 | `icon-compare-1.jpg` | 多个系统图标的细微玻璃更新 | 小尺寸图标的材质变化应克制，轮廓和颜色识别更重要。 |
| R39 | `icon-compare-3.jpg` | 系统工具图标也被重新抽象 | 抽象化必须通过真实可用性测试，不能仅为适配新容器而更换熟悉隐喻。 |
| R40 | `tahoe-folder-color-change.webp` | Finder 文件夹颜色可随强调色和标签变化 | 个性化颜色需要受控集合、即时预览和恢复默认。 |
| R41 | `squircle-jail.jpg` | 非圆角方形应用图标被塞入灰色圆角容器 | 这是反例。不要用统一外壳惩罚合法品牌形状。 |
| R42 | `icon-composer.jpeg` | Icon Composer 分层编辑、主题和背景预览 | 图标资产应在浅色、暗色、透明和多种背景上预览，并在构建阶段生成变体。 |
| R43 | `tahoe-app-pop-in.webp` | Finder 打开后图标延迟逐个出现 | 主题化与合成结果需要缓存，运行时不应引入明显 Pop-in。 |

### A.4 Spotlight、自动化与 Web 能力

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R44 | `tahoe-spotlight.jpeg` | 居中的 Spotlight 搜索与结果 | 命令面板应快速聚焦、宽度稳定、结果可键盘遍历。 |
| R45 | `spotlight-primer.jpeg` | Spotlight 新能力总览 | 首次引导只解释模式和关键动作，不用长教程覆盖主界面。 |
| R46 | `spotlight-apps.jpeg` | Applications 模式 | 同类对象使用图标网格时仍需名称、来源和清楚的选中状态。 |
| R47 | `spotlight-view-options.jpeg` | 网格与列表视图切换 | 用图标分段控件表示熟悉的视图模式，并提供 Tooltip。 |
| R48 | `spotlight-apps-list.jpeg` | Applications 列表模式 | 高数量对象优先列表和键盘扫描，网格不是唯一答案。 |
| R49 | `spotlight-action-2.jpeg` | Actions 与 Quick Keys | 动作与对象分开，快捷键作为加速器而不是唯一入口。 |
| R50 | `spotlight-clipboard.jpeg` | 剪贴板历史 | 默认关闭、保留期明确、可搜索、可清除，敏感数据需要特别处理。 |
| R51 | `spotlight-settings.jpeg` | Spotlight 新设置页 | 高风险设置与普通搜索设置分组，并提供清除历史和恢复操作。 |
| R52 | `shortcuts-automation-2.jpeg` | 自动化连接 NAS 和朗读电量的示例 | 自动化列表需要清楚显示触发器、动作、启用状态和最近结果。 |
| R53 | `shortcuts-automate.jpeg` | 新建自动化配置 | 配置流程按触发器、条件、动作、确认和通知分步组织。 |
| R54 | `automation-confirmation.jpeg` | 自动化执行前通过通知确认 | 对有副作用的自动化提供确认模式，并让通知直接给出允许与取消。 |
| R55 | `webgl-sonoma-safari.jpeg` | Safari WebGPU 跨系统兼容性测试 | 视觉增强和高级 API 必须能力检测，并有无 WebGPU 的完整回退。 |
| R56 | `bad-typography-SM-dark.png` | `text-wrap: pretty` 要改善的孤行、参差和文字河流 | 长说明可使用更好的换行算法，但必须控制范围和性能。 |

### A.5 系统应用

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R57 | `tahoe-phone-dialer.jpeg` | Phone 拨号盘 | 宽屏仍可保留熟悉的数字网格，键盘输入和粘贴必须同样可用。 |
| R58 | `phone-app-notfication.jpg` | 通话通知与快速控制 | 持续任务通知要显示对象、状态、时长和最关键操作，不堆满功能。 |
| R59 | `tahoe-games-app.jpeg` | Games 首页与内容聚合 | 媒体型内容可以更丰富，但导航、来源和当前分类必须稳定。 |
| R60 | `tahoe-games-library.jpeg` | 跨 App Store 与其他来源的游戏库 | 聚合列表需要来源、安装状态、兼容性和筛选，不能只靠封面。 |
| R61 | `messages-app.jpeg` | Messages 背景与半透明对话气泡 | 用户背景属于高风险自定义。气泡可透明，名称、时间和系统状态必须有稳定背板。 |
| R62 | `messages-background-2.jpeg` | Details 面板从右侧滑入 | Inspector/Drawer 能保留上下文，适合客户端详情、连接详情和配置检查。 |
| R63 | `messages-background.jpeg` | 内置背景、颜色和照片选择 | 预设优先，实时预览，说明背景会同步或影响其他参与者。 |
| R64 | `notes-markdown.jpeg` | Notes 导入导出 Markdown | 格式转换要明确支持范围、预览差异和可能丢失的能力。 |
| R65 | `tahoe-terminal.jpeg` | Terminal Clear Light/Dark、SF Mono Terminal 与更大默认窗口 | 专业工具的可读字体、字符覆盖和默认密度优先，透明度只作轻微增强。 |
| R66 | `tahoe-passwords-export.jpeg` | Passwords 安全导入导出 Passkey | 敏感迁移要直接、加密、可确认，不通过明文中间文件。 |

### A.6 锁屏、持续状态、系统反馈与恢复

| ID | 文件 | 画面与观察点 | 可复用结论 |
| --- | --- | --- | --- |
| R67 | `tahoe-lock-screen.jpg` | 锁屏时钟字体和粗细选择 | 个性化选项保持少量高质量预设，位置应容易发现。 |
| R68 | `live-activities-tahoe.jpg` | iPhone Live Activity 显示在 Mac 菜单栏 | 持续状态适合紧凑胶囊，只显示对象、进度或预计时间和一个入口。 |
| R69 | `tahoe-game-overlay.jpeg` | 游戏中的上下文覆盖层 | 把当前任务相关控制集中在局部 Overlay，不强迫跳出当前场景。 |
| R70 | `sound-indicator.jpeg` | 新音量提示从菜单栏附近弹出 | 瞬时反馈靠近触发源，减少中心遮挡，出现时间短。 |
| R71 | `brightness-indicator.jpeg` | 与音量提示一致的亮度反馈 | 同类系统指标复用同一组件，只替换图标、数值和语义。 |
| R72 | `tahoe-low-battery.jpeg` | 低电量通知与红色圆形电量计 | 紧急状态同时使用数字、图形、颜色和明确行动，频率应可控。 |
| R73 | `tahoe-recovery.jpeg` | Recovery 中的 Device Recovery Assistant | 恢复流程需要显示步骤、进度、结果和下一步，不能只给模糊的“正在检查”。 |
| R74 | `tahoe-recovery-browser.jpeg` | 精简的恢复环境 Web Browser | 紧急环境只保留任务必需功能，去掉书签、品牌装饰和无关设置。 |

## 附录 B：参考来源

### 核心评测

- [Ars Technica: macOS 26 Tahoe review](https://arstechnica.com/gadgets/2025/09/macos-26-tahoe-the-ars-technica-review/)

### Apple 官方资料

- [Apple Human Interface Guidelines: Materials](https://developer.apple.com/design/human-interface-guidelines/materials)
- [Apple Developer: Liquid Glass](https://developer.apple.com/documentation/TechnologyOverviews/liquid-glass)
- [Apple Developer: Adopting Liquid Glass](https://developer.apple.com/documentation/TechnologyOverviews/adopting-liquid-glass)
- [Apple Developer: SwiftUI Material](https://developer.apple.com/documentation/SwiftUI/Material)

### Web 标准资料

- [MDN: backdrop-filter](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/Properties/backdrop-filter)
- [MDN: prefers-color-scheme](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-color-scheme)
- [MDN: prefers-reduced-motion](https://developer.mozilla.org/en-US/docs/Web/CSS/Reference/At-rules/@media/prefers-reduced-motion)
- [WCAG 2.2](https://www.w3.org/TR/WCAG22/)

## 附录 C：维护约定

- 本文使用语义 token，不以具体项目类名作为长期接口。
- 修改颜色时同时更新浅色、暗色、强制颜色和降低透明度四种模式。
- 新增组件时先说明其层级、状态、键盘行为和回退，再讨论玻璃效果。
- 设计验收以可读性、状态准确性和操作效率为先，像素相似度为后。
- 若未来 Apple 调整 Tahoe 材质，应先验证其是否改善 Web 产品问题，再决定是否跟随。
