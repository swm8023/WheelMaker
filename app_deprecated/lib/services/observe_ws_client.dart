import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../data/project_data_source.dart';

class ObserveWsClient {
  final WebSocketChannel _channel;
  final String? _token;
  final Duration _timeout;
  int _seq = 0;
  final Map<String, Completer<Map<String, dynamic>>> _pending = {};
  late final StreamSubscription<dynamic> _sub;

  ObserveWsClient._(
    this._channel, {
    required String? token,
    required Duration timeout,
  })  : _token = token,
        _timeout = timeout {
    _sub = _channel.stream.listen(_onData, onError: _onError, onDone: _onDone);
  }

  static Future<ObserveWsClient> connect({
    required String address,
    String? token,
    Duration timeout = const Duration(seconds: 8),
  }) async {
    final channel = WebSocketChannel.connect(Uri.parse(address));
    await channel.ready;
    return ObserveWsClient._(
      channel,
      token: token?.isEmpty == true ? null : token,
      timeout: timeout,
    );
  }

  Future<void> hello() async {
    await request(
      method: 'hello',
      payload: const {
        'clientName': 'wheelmaker-app',
        'clientVersion': '0.1.0',
        'protocolVersion': '1.0',
      },
      includeProjectId: false,
    );
    if (_token != null) {
      await request(
        method: 'auth',
        payload: {'token': _token},
        includeProjectId: false,
      );
    }
  }

  Future<List<ProjectDescriptor>> projectList() async {
    final resp = await request(
      method: 'project.list',
      payload: const {},
      includeProjectId: false,
    );
    final payload = resp['payload'] as Map<String, dynamic>? ?? const {};
    final raw = payload['projects'] as List<dynamic>? ?? const [];
    return raw
        .whereType<Map<String, dynamic>>()
        .map(
          (item) => ProjectDescriptor(
            id: (item['projectId'] as String?) ?? '',
            name: (item['name'] as String?) ??
                ((item['projectId'] as String?) ?? ''),
          ),
        )
        .where((p) => p.id.isNotEmpty)
        .toList();
  }

  Future<List<ObserveFsEntry>> fsList(String projectId, String path) async {
    final resp = await request(
      method: 'fs.list',
      projectId: projectId,
      payload: {
        'path': path,
        'cursor': '',
        'limit': 200,
      },
    );
    final payload = resp['payload'] as Map<String, dynamic>? ?? const {};
    final raw = payload['entries'] as List<dynamic>? ?? const [];
    return raw
        .whereType<Map<String, dynamic>>()
        .map((item) {
          final entryPath = (item['path'] as String?) ?? '';
          return ObserveFsEntry(
            name: (item['name'] as String?) ?? '',
            path: entryPath,
            isDir: (item['kind'] as String?) == 'dir',
          );
        })
        .where((e) => e.path.isNotEmpty && e.name.isNotEmpty)
        .toList();
  }

  Future<String> fsRead(String projectId, String path) async {
    final resp = await request(
      method: 'fs.read',
      projectId: projectId,
      payload: {
        'path': path,
        'offset': 0,
        'limit': 65536,
      },
    );
    final payload = resp['payload'] as Map<String, dynamic>? ?? const {};
    return (payload['content'] as String?) ?? '';
  }

  Future<Map<String, dynamic>> request({
    required String method,
    required Map<String, dynamic> payload,
    String? projectId,
    bool includeProjectId = true,
  }) async {
    final id = 'req-${_seq++}';
    final completer = Completer<Map<String, dynamic>>();
    _pending[id] = completer;
    final msg = <String, dynamic>{
      'version': '1.0',
      'requestId': id,
      'type': 'request',
      'method': method,
      'payload': payload,
    };
    if (includeProjectId && projectId != null && projectId.isNotEmpty) {
      msg['projectId'] = projectId;
    }
    _channel.sink.add(jsonEncode(msg));
    return completer.future.timeout(_timeout, onTimeout: () {
      _pending.remove(id);
      throw TimeoutException('Observe request timed out: $method');
    });
  }

  void _onData(dynamic raw) {
    if (raw is! String) return;
    final data = jsonDecode(raw) as Map<String, dynamic>;
    final requestId = data['requestId'] as String?;
    if (requestId == null) return;
    final pending = _pending.remove(requestId);
    if (pending == null) return;
    if (data['type'] == 'error') {
      final error = data['error'] as Map<String, dynamic>? ?? const {};
      pending.completeError(Exception(error['message'] ?? 'observe error'));
      return;
    }
    pending.complete(data);
  }

  void _onError(Object error) {
    for (final entry in _pending.values) {
      if (!entry.isCompleted) {
        entry.completeError(error);
      }
    }
    _pending.clear();
  }

  void _onDone() {
    _onError(StateError('observe connection closed'));
  }

  Future<void> close() async {
    await _sub.cancel();
    await _channel.sink.close();
  }
}

class ObserveFsEntry {
  final String name;
  final String path;
  final bool isDir;

  const ObserveFsEntry({
    required this.name,
    required this.path,
    required this.isDir,
  });
}
