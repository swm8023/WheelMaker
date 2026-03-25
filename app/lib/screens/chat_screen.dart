import 'dart:async';
import 'dart:io' show Platform;
import 'dart:math' show min;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../models/chat_message.dart';
import '../services/ws_service.dart';
import 'connect_screen.dart';

/// Main chat screen — shows message history, handles input, option buttons,
/// and connection-state indicator.
class ChatScreen extends StatefulWidget {
  final WsService service;
  const ChatScreen({super.key, required this.service});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  final _focusNode = FocusNode();

  final List<ChatMessage> _messages = [];
  late final StreamSubscription<ChatMessage> _msgSub;
  late final StreamSubscription<WsState> _stateSub;

  WsState _wsState = WsState.connecting;
  bool _isWaiting = false; // true while waiting for agent response

  bool get _isDesktop =>
      !kIsWeb && (Platform.isWindows || Platform.isMacOS || Platform.isLinux);

  @override
  void initState() {
    super.initState();
    _wsState = widget.service.state;
    _messages.addAll(widget.service.initialMessages);
    _msgSub = widget.service.messages.listen(_onMessage);
    _stateSub = widget.service.stateStream.listen(_onStateChange);
    // On desktop: Enter sends, Shift+Enter inserts newline.
    if (_isDesktop) {
      _focusNode.onKeyEvent = (_, event) {
        if (event is KeyDownEvent &&
            event.logicalKey == LogicalKeyboardKey.enter &&
            !HardwareKeyboard.instance.isShiftPressed) {
          _send();
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      };
    }
  }

  @override
  void dispose() {
    _msgSub.cancel();
    _stateSub.cancel();
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    _focusNode.dispose();
    widget.service.dispose();
    super.dispose();
  }

  void _onMessage(ChatMessage msg) {
    setState(() {
      _messages.add(msg);
      // Any agent/system message clears the waiting indicator.
      if (msg.type == MessageType.agent ||
          msg.type == MessageType.system ||
          msg.type == MessageType.options) {
        _isWaiting = false;
      }
    });
    _scrollToBottom();
  }

  void _onStateChange(WsState s) {
    setState(() => _wsState = s);
    if (s == WsState.disconnected || s == WsState.error) {
      _showDisconnectBanner();
    }
  }

  void _showDisconnectBanner() {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showMaterialBanner(
      MaterialBanner(
        content: Text(
          _wsState == WsState.error ? 'Connection error' : 'Disconnected',
        ),
        actions: [
          TextButton(
            onPressed: () {
              ScaffoldMessenger.of(context).hideCurrentMaterialBanner();
              Navigator.pushReplacement(
                context,
                MaterialPageRoute(builder: (_) => const ConnectScreen()),
              );
            },
            child: const Text('Reconnect'),
          ),
        ],
      ),
    );
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) {
        _scrollCtrl.animateTo(
          _scrollCtrl.position.maxScrollExtent,
          duration: const Duration(milliseconds: 250),
          curve: Curves.easeOut,
        );
      }
    });
  }

  void _send() {
    final text = _inputCtrl.text.trim();
    if (text.isEmpty || _wsState != WsState.ready) return;
    setState(() {
      _messages.add(ChatMessage.user(text));
      _isWaiting = true;
    });
    widget.service.sendMessage(text);
    _inputCtrl.clear();
    _focusNode.requestFocus();
    _scrollToBottom();
  }

  // ── State indicator ─────────────────────────────────────────────────────────

  Color _stateColor() => switch (_wsState) {
        WsState.ready => Colors.greenAccent,
        WsState.error || WsState.disconnected => Colors.redAccent,
        _ => Colors.orangeAccent,
      };

  String _stateLabel() => switch (_wsState) {
        WsState.ready => 'Online',
        WsState.connecting => 'Connecting…',
        WsState.authenticating => 'Authenticating…',
        WsState.error => 'Error',
        WsState.disconnected => 'Offline',
      };

  // ── Build ────────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('WheelMaker'),
        actions: [
          _StatusDot(color: _stateColor(), label: _stateLabel()),
          const SizedBox(width: 8),
        ],
      ),
      // LayoutBuilder lets us center content on wide screens (tablet/desktop)
      // while keeping full-width layout on phones.
      body: LayoutBuilder(
        builder: (context, constraints) {
          final double hPad =
              constraints.maxWidth > 900 ? (constraints.maxWidth - 900) / 2 : 0.0;
          return Padding(
            padding: EdgeInsets.symmetric(horizontal: hPad),
            child: Column(
              children: [
                Expanded(
                  child: ListView.builder(
                    controller: _scrollCtrl,
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                    itemCount: _messages.length + (_isWaiting ? 1 : 0),
                    itemBuilder: (ctx, i) {
                      if (i == _messages.length) return const _TypingIndicator();
                      return _buildMessage(ctx, _messages[i]);
                    },
                  ),
                ),
                _buildInputBar(),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _buildMessage(BuildContext ctx, ChatMessage msg) {
    switch (msg.type) {
      case MessageType.system:
        return Padding(
          padding: const EdgeInsets.symmetric(vertical: 6),
          child: Center(
            child: Text(
              msg.text,
              style: TextStyle(
                color: Theme.of(ctx).colorScheme.onSurfaceVariant,
                fontSize: 12,
              ),
            ),
          ),
        );

      case MessageType.debug:
        return Container(
          margin: const EdgeInsets.symmetric(vertical: 2),
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: Colors.amber.withOpacity(0.08),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Text(
            '[debug] ${msg.text}',
            style: const TextStyle(fontFamily: 'monospace', fontSize: 11),
          ),
        );

      case MessageType.options:
        return Card(
          margin: const EdgeInsets.symmetric(vertical: 6),
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                if (msg.title != null && msg.title!.isNotEmpty)
                  Text(
                    msg.title!,
                    style: const TextStyle(fontWeight: FontWeight.bold),
                  ),
                if (msg.text.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 6, bottom: 10),
                    child: Text(msg.text),
                  ),
                Wrap(
                  spacing: 8,
                  runSpacing: 6,
                  children: msg.options
                      .map((o) => FilledButton.tonal(
                            onPressed: () => msg.onSelected?.call(o.id),
                            child: Text(o.label),
                          ))
                      .toList(),
                ),
              ],
            ),
          ),
        );

      case MessageType.user:
        return _Bubble(
          text: msg.text,
          isUser: true,
          onLongPress: () => _copyToClipboard(msg.text),
        );

      case MessageType.agent:
        return _Bubble(
          text: msg.text,
          isUser: false,
          onLongPress: () => _copyToClipboard(msg.text),
        );
    }
  }

  void _copyToClipboard(String text) {
    Clipboard.setData(ClipboardData(text: text));
    if (mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Copied'), duration: Duration(seconds: 1)));
    }
  }

  Widget _buildInputBar() {
    // No safe-area bottom padding needed on desktop.
    final bottom = _isDesktop ? 0.0 : MediaQuery.of(context).padding.bottom;
    return Container(
      padding: EdgeInsets.fromLTRB(12, 8, 8, 8 + bottom),
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: Theme.of(context).dividerColor)),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          Expanded(
            child: TextField(
              controller: _inputCtrl,
              focusNode: _focusNode,
              minLines: 1,
              maxLines: 5,
              textInputAction: TextInputAction.newline,
              keyboardType: TextInputType.multiline,
              decoration: const InputDecoration(
                hintText: 'Message…',
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.all(Radius.circular(24)),
                ),
                contentPadding: EdgeInsets.symmetric(horizontal: 16, vertical: 10),
              ),
            ),
          ),
          const SizedBox(width: 6),
          IconButton.filled(
            onPressed: _wsState == WsState.ready ? _send : null,
            icon: const Icon(Icons.send_rounded),
            tooltip: 'Send',
          ),
        ],
      ),
    );
  }
}

// ── Small reusable widgets ────────────────────────────────────────────────────

class _StatusDot extends StatelessWidget {
  final Color color;
  final String label;
  const _StatusDot({required this.color, required this.label});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 9,
          height: 9,
          decoration: BoxDecoration(shape: BoxShape.circle, color: color),
        ),
        const SizedBox(width: 5),
        Text(label, style: TextStyle(fontSize: 12, color: color)),
      ],
    );
  }
}

class _Bubble extends StatelessWidget {
  final String text;
  final bool isUser;
  final VoidCallback? onLongPress;

  const _Bubble({required this.text, required this.isUser, this.onLongPress});

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    final bgColor = isUser ? cs.primary : cs.surfaceContainerHighest;
    final align = isUser ? Alignment.centerRight : Alignment.centerLeft;
    final radius = BorderRadius.only(
      topLeft: const Radius.circular(18),
      topRight: const Radius.circular(18),
      bottomLeft: Radius.circular(isUser ? 18 : 4),
      bottomRight: Radius.circular(isUser ? 4 : 18),
    );

    return Align(
      alignment: align,
      child: GestureDetector(
        onLongPress: onLongPress,
        child: Container(
          margin: const EdgeInsets.symmetric(vertical: 3),
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          constraints: BoxConstraints(
            // Cap bubble width so it stays readable on wide screens.
            maxWidth: min(MediaQuery.of(context).size.width * 0.78, 600),
          ),
          decoration: BoxDecoration(color: bgColor, borderRadius: radius),
          child: SelectableText(text),
        ),
      ),
    );
  }
}

class _TypingIndicator extends StatefulWidget {
  const _TypingIndicator();

  @override
  State<_TypingIndicator> createState() => _TypingIndicatorState();
}

class _TypingIndicatorState extends State<_TypingIndicator>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.symmetric(vertical: 3),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: cs.surfaceContainerHighest,
          borderRadius: const BorderRadius.only(
            topLeft: Radius.circular(18),
            topRight: Radius.circular(18),
            bottomRight: Radius.circular(18),
            bottomLeft: Radius.circular(4),
          ),
        ),
        child: AnimatedBuilder(
          animation: _ctrl,
          builder: (_, __) {
            final v = _ctrl.value;
            return Row(
              mainAxisSize: MainAxisSize.min,
              children: List.generate(3, (i) {
                final offset = (v - i * 0.15).clamp(0.0, 1.0);
                final opacity = (offset < 0.5 ? offset : 1.0 - offset) * 2;
                return Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 3),
                  child: Opacity(
                    opacity: opacity.clamp(0.2, 1.0),
                    child: Container(
                      width: 8,
                      height: 8,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: cs.onSurfaceVariant,
                      ),
                    ),
                  ),
                );
              }),
            );
          },
        ),
      ),
    );
  }
}
