import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../services/ws_service.dart';
import 'chat_screen.dart';

/// Entry screen: lets the user configure server address and optional auth token.
class ConnectScreen extends StatefulWidget {
  const ConnectScreen({super.key});

  @override
  State<ConnectScreen> createState() => _ConnectScreenState();
}

class _ConnectScreenState extends State<ConnectScreen> {
  final _addrCtrl = TextEditingController();
  final _tokenCtrl = TextEditingController();
  bool _connecting = false;

  @override
  void initState() {
    super.initState();
    _loadPrefs();
  }

  @override
  void dispose() {
    _addrCtrl.dispose();
    _tokenCtrl.dispose();
    super.dispose();
  }

  Future<void> _loadPrefs() async {
    final prefs = await SharedPreferences.getInstance();
    setState(() {
      _addrCtrl.text = prefs.getString('wm_addr') ?? 'ws://192.168.1.100:9527/ws';
      _tokenCtrl.text = prefs.getString('wm_token') ?? '';
    });
  }

  Future<void> _connect() async {
    final addr = _addrCtrl.text.trim();
    final token = _tokenCtrl.text.trim();
    if (addr.isEmpty) return;

    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('wm_addr', addr);
    await prefs.setString('wm_token', token);

    setState(() => _connecting = true);

    final svc = WsService();
    await svc.connect(addr, token);

    if (!mounted) {
      svc.dispose();
      return;
    }
    setState(() => _connecting = false);

    if (svc.state == WsState.error || svc.state == WsState.disconnected) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Connection failed — check address and token')),
      );
      svc.dispose();
      return;
    }

    Navigator.pushReplacement(
      context,
      MaterialPageRoute(builder: (_) => ChatScreen(service: svc)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final cs = Theme.of(context).colorScheme;
    return Scaffold(
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Align(
            alignment: Alignment.topCenter,
            child: ConstrainedBox(
              // Limit form width on tablet / desktop for readability.
              constraints: const BoxConstraints(maxWidth: 440),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const SizedBox(height: 48),
              Icon(Icons.terminal_rounded, size: 64, color: cs.primary),
              const SizedBox(height: 16),
              Text(
                'WheelMaker',
                textAlign: TextAlign.center,
                style: Theme.of(context)
                    .textTheme
                    .headlineLarge
                    ?.copyWith(fontWeight: FontWeight.bold),
              ),
              const SizedBox(height: 6),
              Text(
                'Remote AI coding assistant',
                textAlign: TextAlign.center,
                style: TextStyle(color: cs.onSurfaceVariant),
              ),
              const SizedBox(height: 48),
              TextField(
                controller: _addrCtrl,
                enabled: !_connecting,
                decoration: const InputDecoration(
                  labelText: 'Server Address',
                  hintText: 'ws://192.168.1.x:9527/ws',
                  border: OutlineInputBorder(),
                  prefixIcon: Icon(Icons.wifi),
                ),
                keyboardType: TextInputType.url,
                textInputAction: TextInputAction.next,
              ),
              const SizedBox(height: 16),
              TextField(
                controller: _tokenCtrl,
                enabled: !_connecting,
                decoration: const InputDecoration(
                  labelText: 'Token (leave blank if not required)',
                  border: OutlineInputBorder(),
                  prefixIcon: Icon(Icons.key),
                ),
                obscureText: true,
                textInputAction: TextInputAction.done,
                onSubmitted: (_) => _connect(),
              ),
              const SizedBox(height: 24),
              FilledButton.icon(
                onPressed: _connecting ? null : _connect,
                icon: _connecting
                    ? const SizedBox(
                        width: 18,
                        height: 18,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.link),
                label: Text(_connecting ? 'Connecting…' : 'Connect'),
              ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
