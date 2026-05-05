# Session Picker 与 README 刷新设计

## 背景

当前 Web App 中的 `New Session` 与 `Resume Session` 已经都采用弹层式入口，但两者在头部结构、关闭方式、操作层级、列表样式上仍不统一，用户会感知为两套相近但不一致的流程。与此同时，`README.md` 仍主要是工程型说明，无法直接服务首次部署用户，也没有完整体现 App 已具备的聊天、文件、Git、渲染、连接恢复、PWA 与观测能力。

本次工作目标是同时解决这两个问题：

1. 把 `new session` 与 `resume session` 统一为同一套会话选择器体验。
2. 把 `README.md` 重写为以“首次自建使用者”为主、以“功能导览”为辅的文档，并补齐当前网络环境下的 Nginx + HTTPS 示例与可视化说明。

## 目标

- `New Session` 与 `Resume Session` 使用同一套弹层骨架、头部、关闭入口、返回按钮、操作按钮与卡片列表视觉语言。
- 保留两条流程的行为差异，不把二者硬合成一个复杂状态机。
- `README.md` 顶层拆成“使用”和“功能”两部分。
- “使用”部分提供贴近当前环境的双机示例：机器 A 上同时承载 hub、registry、monitor、web 与 Nginx；机器 B 上运行另一个 hub 并接入机器 A 的 registry。
- “功能”部分覆盖 App 的实际能力，而不是只列基础卖点。
- README 中补充拓扑图、访问路径图、界面示意图；图示资产纳入仓库管理。

## 非目标

- 不重做整个 chat sidebar 的信息架构。
- 不引入新的运行时依赖、文档站点或图片构建系统。
- 不修改 registry / monitor / hub 的协议和端口行为，只做文档化与界面一致性整理。

## 一、Session Picker 统一设计

### 1.1 结构统一

`New Session` 与 `Resume Session` 统一为“会话选择器”外观：

- 同一层级的弹层卡片容器；
- 同一头部结构：左侧图标 + 标题，右上角关闭按钮；
- 同一说明文案区域；
- 同一按钮组样式；
- 同一列表项卡片样式；
- 同一 loading / empty / disabled 状态表现。

两条流程共享“骨架层”，但内部内容按模式切换：

- `new` 模式：显示 agent 选择列表，用于创建新 session；
- `resume` 模式：
  - 初始显示 agent 选择列表；
  - 选定 agent 后显示返回按钮和可恢复 session 列表；
  - 点击某个 session 后执行 import → reload → load history。

### 1.2 交互约束

- 两种 picker 都必须有显式的右上角关闭按钮。
- 只有存在“上一步”时才显示返回按钮，不使用占位空白。
- agent 选择按钮、session 项按钮、关闭按钮、返回按钮全部采用统一 hover / disabled / focus 视觉。
- `resume` 流程保留已修复行为：若 import 成功但后续 reload 或历史加载失败，不再把该 session 留在“可重复 import”的列表里。

### 1.3 实现方式

不新增大而散的文件，继续在 `app/web/src/main.tsx` 与 `app/web/src/styles.css` 内完成，但抽出小型共享结构，减少 JSX 与样式复制。允许采用以下方式之一：

- 共享 render helper；
- 共享 props 片段；
- 共享 CSS class 组合。

约束是：共享的是“骨架与视觉语言”，不是把 `new` / `resume` 的业务状态机混成一个大函数。

### 1.4 测试策略

继续沿用当前 Web 侧源码断言测试风格，补充或调整针对 picker 统一性的断言，覆盖：

- `new session` 使用和 `resume session` 一致的 overlay/card/header/close 结构；
- `resume session` 保留 back + import + reload + full history load 行为；
- 公共样式类存在并被两条流程使用。

## 二、README 重构设计

### 2.1 顶层结构

`README.md` 重构为以下主结构：

1. 项目简介
2. 使用
3. 功能
4. 仓库结构 / 开发（保留但后置）

其中“使用”为主线，“功能”为能力导览。

### 2.2 使用部分

“使用”部分面向第一次部署用户，按从外到内的顺序组织：

1. **部署场景**
   - 机器 A：hub + registry + monitor + web + Nginx
   - 机器 B：hub，连接机器 A 的 registry
2. **拓扑图**
   - 用户浏览器 / 手机 → Nginx(机器 A)
   - Nginx → web(`/`) / registry WebSocket(`/ws`) / monitor(`/monitor/`)
   - 机器 B hub → registry
3. **端口与入口表**
   - 基于当前 Nginx 配置说明：
     - `28800`：web + `/ws` + `/monitor/`
     - `9630`：registry 内部监听
     - `9631`：monitor 内部监听
     - `33006 -> 3006`：另一组 HTTPS 反代示例
4. **配置说明**
   - `config.json` 中 `projects`、`registry.listen`、`registry.server`、`registry.token`、`registry.hubId`、`monitor.port`
   - 机器 A / B 的配置差异
5. **Nginx + HTTPS 示例**
   - 参考当前 `D:\Nginx\nginx-1.29.5\conf\conf.d\proxy-28800.conf`
   - 解释 `/`、`/ws`、`/monitor/` 的代理角色
6. **访问与检查**
   - 浏览器打开 Web / Monitor
   - hub 接入 registry 的方式
   - 最短健康检查指引

### 2.3 功能部分

“功能”部分必须覆盖 App 的实际用户能力，按用户视角组织，而不是只写底层能力名词。规划为以下分组：

1. **聊天与会话**
   - 新建 session
   - 恢复 session
   - 会话列表与切换
   - 增量同步与强制重载
2. **项目与工作区**
   - 多项目切换
   - 多 hub 聚合
   - 在线状态与当前 agent 展示
3. **文件浏览与阅读**
   - 目录树
   - 文件打开 / 缓存 / not-modified
   - pinned files
   - 文件滚动位置恢复
4. **Git 浏览**
   - branch / dirty 状态
   - staged / unstaged / untracked
   - commit 列表、commit 文件、diff
   - working tree diff
5. **富内容渲染**
   - Markdown
   - 表格 / 数学公式 / Mermaid
   - 代码高亮与 diff 预览
   - 主题 / 字体 / 代码显示设置
6. **连接恢复与 PWA**
   - 自动重连
   - 工作区保持可见
   - 前后台恢复
   - 本地通知
7. **设置与观测**
   - runtime / registry 地址
   - token provider / token stats
   - monitor 页面

每一组都至少配一句“能做什么”的说明，以及一张图示或界面图。

## 三、README 图示资产设计

### 3.1 资产形式

优先使用仓库内可维护的静态 SVG 资产，避免引入额外截图流程依赖。图示分为三类：

- 拓扑图；
- 流程图；
- 界面示意图。

如当前环境方便获取真实截图，可混合使用；若不方便，则界面图以高保真 SVG 示意图替代。文档必须在没有外部截图工具参与的前提下仍然完整可交付。

### 3.2 资产目录

新增 README 资源目录，例如：

- `docs/readme-assets/topology-*.svg`
- `docs/readme-assets/ui-*.svg`

命名需直接反映用途，便于后续扩展。

### 3.3 图示清单

至少提供以下图：

1. 双机部署拓扑图；
2. Nginx 访问路径图；
3. Chat / File / Git 主界面示意图；
4. 统一后的 Session Picker 示意图；
5. Monitor 界面示意图。

## 四、验证要求

### 4.1 Web

- 受影响的源码断言测试通过；
- `npm run tsc:web` 通过；
- 如新增 README 资源路径，确认引用路径有效。

### 4.2 文档

- README 结构清晰，一级章节稳定；
- Nginx 示例与当前配置一致，不虚构不存在的入口路径；
- 机器 A / B 示例中 registry / monitor / web 的角色关系前后一致。

## 五、风险与控制

### 风险 1：UI 统一过程中把业务流程混乱化

控制方式：只抽共享骨架，不抽共享状态机；保留 `new` 与 `resume` 各自事件处理函数。

### 风险 2：README 功能描述脱离实际实现

控制方式：以现有 `main.tsx`、`registryWorkspaceService.ts`、现有 Web 测试名与行为为依据归纳功能，不写代码中不存在的能力。

### 风险 3：文档图示难以维护

控制方式：全部使用仓库内 SVG，优先结构化示意图而不是外部截图资源依赖。
