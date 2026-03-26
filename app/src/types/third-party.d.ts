declare module 'react-native-syntax-highlighter' {
  import * as React from 'react';

  const SyntaxHighlighter: React.ComponentType<Record<string, unknown>>;
  export default SyntaxHighlighter;
}

declare module 'react-syntax-highlighter/styles/hljs' {
  export const vs2015: Record<string, unknown>;
  export const vs: Record<string, unknown>;
}
