# WheelMaker — App

Flutter 移动端 App（iOS / Android），通过 WebSocket 连接到本地 WheelMaker Go 守护进程，提供远程控制 AI CLI 的聊天界面。

## 目录结构

```
app/
  lib/
    main.dart                  — 入口，Material 3 深色主题
    models/
      chat_message.dart        — ChatMessage / MessageType / OptionItem
    services/
      ws_service.dart          — WebSocket 连接管理、协议收发
    screens/
      connect_screen.dart      — 服务器地址 + Token 输入页
      chat_screen.dart         — 主聊天界面
  scripts/
    init-app.ps1               — 首次生成 Android/iOS 平台文件
  pubspec.yaml
```

## 依赖

| 包 | 用途 |
|----|------|
| `web_socket_channel ^3.0.1` | WebSocket 通信 |
| `shared_preferences ^2.3.0` | 持久化服务器地址 / Token |

## 连接与消息流

1. `ConnectScreen` → 用户输入 `ws://host:9527` + token → `WsService.connect()`
2. `WsService` 握手：收到 `auth_required` → 发 `auth` → 收到 `ready` → 进入就绪态
3. `ChatScreen` 订阅 `WsService.messages` 流渲染气泡；订阅 `stateStream` 显示状态点
4. 用户发消息 → `sendMessage(text)` → `{type:"message", text}`
5. 服务端推送 `options`（携带 `decisionId`）→ 渲染选项按钮 → 点击 → `selectOption(decisionId, optionId)`

## WsState 状态机

```
disconnected → connecting → authenticating → ready → disconnected
                                           ↘ error
```

## MessageType 枚举

| 类型 | 显示样式 |
|------|----------|
| `user` | 右对齐气泡（primary 色） |
| `agent` | 左对齐气泡（surfaceContainerHighest） |
| `system` | 居中灰色小字 |
| `debug` | 琥珀色等宽字体 |
| `options` | Card + FilledButton.tonal 选项列表 |

## 开发约定

- 代码注释和标识符用英文
- **每次改完自动 commit + push**
- Flutter SDK 最低要求：`>=3.2.0`
- 平台文件（`android/`、`ios/`）通过 `flutter create` 生成，不手写

## 本地开发

```powershell
# 首次初始化平台文件（在 app/ 目录或仓库根均可）
pwsh app/scripts/init-app.ps1

# 运行 / 构建（在 app/ 目录下）
cd app
flutter pub get
flutter run                        # 连接设备调试
flutter build apk --debug          # 调试 APK
flutter build apk --release        # 发布 APK
```

## Key Invariants (do not break)

| # | Invariant |
|---|-----------|
| 1 | `WsService` is the single WebSocket owner — screens never open raw connections |
| 2 | Screen widgets are stateless consumers of `WsService.messages` / `stateStream` |
| 3 | Auth handshake sequence: `auth_required` → send `auth` → wait for `ready` before any message |
| 4 | `options` messages carry `decisionId`; always echo it back in `option` response |
| 5 | Platform dirs (`android/`, `ios/`) are generated — never manually edited or committed |

## 关键协议文档

- Mobile WebSocket 协议详见：[../server/CLAUDE.md](../server/CLAUDE.md)（Mobile WebSocket 协议章节）
- ACP 协议：[../docs/acp-protocol-full.zh-CN.md](../docs/acp-protocol-full.zh-CN.md)
