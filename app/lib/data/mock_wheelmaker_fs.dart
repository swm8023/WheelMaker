import '../models/file_tree_node.dart';

const FileTreeNode mockWheelMakerRoot = FileTreeNode.dir(
  name: 'WheelMaker',
  path: '/WheelMaker',
  children: [
    FileTreeNode.file(
      name: 'CLAUDE.md',
      path: '/WheelMaker/CLAUDE.md',
      content: '''
## Repo Structure
WheelMaker/
  server/
  app/
  docs/
  scripts/

## Global Rules
- Comments and identifiers use English.
- Read subfolder CLAUDE.md before changes.
''',
    ),
    FileTreeNode.dir(
      name: 'app',
      path: '/WheelMaker/app',
      children: [
        FileTreeNode.file(
          name: 'pubspec.yaml',
          path: '/WheelMaker/app/pubspec.yaml',
          content: '''
name: wheelmaker
description: WheelMaker app
version: 1.0.0+1

environment:
  sdk: ">=3.2.0 <4.0.0"

dependencies:
  flutter:
    sdk: flutter
''',
        ),
        FileTreeNode.dir(
          name: 'lib',
          path: '/WheelMaker/app/lib',
          children: [
            FileTreeNode.file(
              name: 'main.dart',
              path: '/WheelMaker/app/lib/main.dart',
              content: '''
import 'package:flutter/material.dart';
import 'screens/connect_screen.dart';

void main() => runApp(const WheelMakerApp());

class WheelMakerApp extends StatelessWidget {
  const WheelMakerApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'WheelMaker',
      home: const ConnectScreen(),
    );
  }
}
''',
            ),
            FileTreeNode.file(
              name: 'screens/chat_screen.dart',
              path: '/WheelMaker/app/lib/screens/chat_screen.dart',
              content: '''
class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});

  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  @override
  Widget build(BuildContext context) {
    return const Scaffold(body: Center(child: Text('Chat')));
  }
}
''',
            ),
          ],
        ),
      ],
    ),
    FileTreeNode.dir(
      name: 'server',
      path: '/WheelMaker/server',
      children: [
        FileTreeNode.file(
          name: 'go.mod',
          path: '/WheelMaker/server/go.mod',
          content: '''
module wheelmaker/server

go 1.23

require (
  github.com/gorilla/websocket v1.5.0
)
''',
        ),
      ],
    ),
    FileTreeNode.dir(
      name: 'scripts',
      path: '/WheelMaker/scripts',
      children: [
        FileTreeNode.file(
          name: 'refresh_flutter_web.ps1',
          path: '/WheelMaker/scripts/refresh_flutter_web.ps1',
          content: '''
param(
  [string]$Device = "edge"
)

Push-Location app
flutter pub get
flutter run -d $Device
Pop-Location
''',
        ),
      ],
    ),
  ],
);
