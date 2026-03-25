import 'package:flutter_test/flutter_test.dart';
import 'package:wheelmaker/screens/code_language.dart';

void main() {
  test('resolves known extensions and falls back to plaintext', () {
    expect(languageFromPath('/a/main.dart'), 'dart');
    expect(languageFromPath('/a/server.go'), 'go');
    expect(languageFromPath('/a/native_bridge.cpp'), 'cpp');
    expect(languageFromPath('/a/native_bridge.hpp'), 'cpp');
    expect(languageFromPath('/a/config.json'), 'json');
    expect(languageFromPath('/a/config.yaml'), 'yaml');
    expect(languageFromPath('/a/README.md'), 'markdown');
    expect(languageFromPath('/a/run.ps1'), 'powershell');
    expect(languageFromPath('/a/file.unknown'), 'plaintext');
  });
}
