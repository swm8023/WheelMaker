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
              name: 'native_bridge.cpp',
              path: '/WheelMaker/app/lib/native_bridge.cpp',
              content: r'''
#include <iostream>
#include <string>
#include <vector>

int main() {
  std::vector<std::string> args{"wheel", "maker"};
  for (const auto& item : args) {
    std::cout << item << std::endl;
  }
  return 0;
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
          name: 'main.go',
          path: '/WheelMaker/server/main.go',
          content: '''
package main

import "fmt"

func main() {
    fmt.Println("WheelMaker server")
}
''',
        ),
        FileTreeNode.file(
          name: 'router.go',
          path: '/WheelMaker/server/router.go',
          content: '''
package main

import "net/http"

func registerRoutes(mux *http.ServeMux) {
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
}
''',
        ),
        FileTreeNode.file(
          name: 'native_agent.cpp',
          path: '/WheelMaker/server/native_agent.cpp',
          content: r'''
#include <map>
#include <string>

int score(const std::string& lang) {
  static const std::map<std::string, int> table{
      {"go", 10},
      {"cpp", 9},
  };
  auto it = table.find(lang);
  return it == table.end() ? 0 : it->second;
}
''',
        ),
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
          content: r'''
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
