import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../models/chat_message.dart';

/// Connection states for the WebSocket session.
enum WsState { disconnected, connecting, authenticating, ready, error }

/// Service that manages the WebSocket connection to the WheelMaker daemon.
///
/// Protocol (App → Daemon):
///   { "type": "auth",    "token": "..." }
///   { "type": "message", "text": "..." }
///   { "type": "option",  "decisionId": "...", "optionId": "..." }
///   { "type": "ping" }
///
/// Protocol (Daemon → App):
///   { "type": "auth_required" }
///   { "type": "ready" }
///   { "type": "text",    "text": "..." }
///   { "type": "card",    "card": {...} }
///   { "type": "options", "decisionId":"...", "title":"...", "body":"...", "options":[...] }
///   { "type": "debug",   "text": "..." }
///   { "type": "error",   "message": "..." }
///   { "type": "pong" }
class WsService {
  WebSocketChannel? _channel;
  WsState _state = WsState.disconnected;
  String? _token;

  final _messageCtrl = StreamController<ChatMessage>.broadcast();
  final _stateCtrl = StreamController<WsState>.broadcast();

  Stream<ChatMessage> get messages => _messageCtrl.stream;
  Stream<WsState> get stateStream => _stateCtrl.stream;
  WsState get state => _state;

  /// Connect to [addr] (e.g. "ws://192.168.1.x:9527/ws").
  /// Returns once the underlying WebSocket handshake succeeds (or fails).
  /// Authentication happens asynchronously; observe [stateStream] for [WsState.ready].
  Future<void> connect(String addr, String token) async {
    _token = token.isEmpty ? null : token;
    _setState(WsState.connecting);

    try {
      _channel = WebSocketChannel.connect(Uri.parse(addr));
      await _channel!.ready;
    } catch (_) {
      _setState(WsState.error);
      return;
    }

    _setState(WsState.authenticating);
    _channel!.stream.listen(
      _onData,
      onError: (_) => _setState(WsState.error),
      onDone: () => _setState(WsState.disconnected),
      cancelOnError: false,
    );
  }

  void _onData(dynamic raw) {
    final msg = jsonDecode(raw as String) as Map<String, dynamic>;
    switch (msg['type']) {
      case 'auth_required':
        if (_token != null) {
          _send({'type': 'auth', 'token': _token});
        } else {
          // No token configured — server may reject.
          _send({'type': 'auth', 'token': ''});
        }

      case 'ready':
        _setState(WsState.ready);
        _messageCtrl.add(ChatMessage.system('Connected ✓'));

      case 'text':
        final text = (msg['text'] as String?) ?? '';
        if (text.isNotEmpty) {
          _messageCtrl.add(ChatMessage.agent(text));
        }

      case 'card':
        // Render card as formatted JSON for now; rich card UI in a future iteration.
        _messageCtrl.add(ChatMessage.agent('[Card]\n${const JsonEncoder.withIndent('  ').convert(msg['card'])}'));

      case 'debug':
        final text = (msg['text'] as String?) ?? '';
        if (text.isNotEmpty) {
          _messageCtrl.add(ChatMessage.debug(text));
        }

      case 'options':
        final rawOpts = (msg['options'] as List<dynamic>?) ?? [];
        final opts = rawOpts
            .cast<Map<String, dynamic>>()
            .map((o) => OptionItem(id: o['id'] ?? '', label: o['label'] ?? ''))
            .where((o) => o.id.isNotEmpty)
            .toList();
        final decisionId = (msg['decisionId'] as String?) ?? '';
        _messageCtrl.add(ChatMessage.options(
          title: (msg['title'] as String?) ?? 'Choose an option',
          body: (msg['body'] as String?) ?? '',
          options: opts,
          onSelected: (optionId) => selectOption(decisionId, optionId),
        ));

      case 'error':
        _setState(WsState.error);
        _messageCtrl.add(ChatMessage.system('Error: ${msg['message'] ?? 'unknown'}'));

      case 'pong':
        break; // heartbeat ack — nothing to do
    }
  }

  /// Send a plain text message to the daemon.
  void sendMessage(String text) {
    _send({'type': 'message', 'text': text});
  }

  /// Resolve a pending decision by selecting [optionId].
  void selectOption(String decisionId, String optionId) {
    _send({'type': 'option', 'decisionId': decisionId, 'optionId': optionId});
  }

  void _send(Map<String, dynamic> msg) {
    _channel?.sink.add(jsonEncode(msg));
  }

  void _setState(WsState s) {
    _state = s;
    _stateCtrl.add(s);
  }

  void disconnect() {
    _channel?.sink.close();
    _setState(WsState.disconnected);
  }

  void dispose() {
    disconnect();
    _messageCtrl.close();
    _stateCtrl.close();
  }
}
