import React from 'react';
import {Platform, ScrollView, StyleSheet, Text, View} from 'react-native';
import Markdown from 'react-native-markdown-display';
import SyntaxHighlighter from 'react-native-syntax-highlighter';
import {vs2015, vs} from 'react-syntax-highlighter/styles/hljs';

import type {AppTheme} from '../theme';

type MarkdownViewProps = {
  content: string;
  theme: AppTheme;
};

type MarkdownNode = {
  key?: string;
  content?: string;
  sourceInfo?: string;
  attributes?: {
    className?: string;
  };
};

function parseFenceLanguage(node: MarkdownNode): string {
  const info = node.sourceInfo?.trim();
  if (info) {
    const fromInfo = info.split(/\s+/)[0]?.toLowerCase();
    if (fromInfo) return fromInfo;
  }

  const className = node.attributes?.className ?? '';
  const match = className.match(/language-([A-Za-z0-9_+-]+)/);
  return match?.[1]?.toLowerCase() ?? 'plaintext';
}

function extractFenceCode(node: MarkdownNode): string {
  return node.content ?? '';
}

export function MarkdownView({content, theme}: MarkdownViewProps) {
  const syntaxStyle = theme.mode === 'dark' ? vs2015 : vs;
  const codeContainerStyle = {
    margin: 0,
    padding: 12,
    backgroundColor: 'transparent',
    minWidth: '100%',
  } as unknown as Record<string, unknown>;

  const markdownStyle = {
    body: {
      color: theme.colors.text,
      fontSize: 14,
      lineHeight: 22,
      fontFamily: theme.font.ui,
    },
    heading1: {
      color: theme.colors.text,
      fontSize: 28,
      fontWeight: '700' as const,
      marginBottom: 10,
      marginTop: 12,
    },
    heading2: {
      color: theme.colors.text,
      fontSize: 22,
      fontWeight: '700' as const,
      marginBottom: 8,
      marginTop: 10,
    },
    heading3: {
      color: theme.colors.text,
      fontSize: 18,
      fontWeight: '600' as const,
      marginBottom: 6,
      marginTop: 8,
    },
    paragraph: {
      marginTop: 0,
      marginBottom: 10,
    },
    bullet_list: {
      marginBottom: 10,
    },
    ordered_list: {
      marginBottom: 10,
    },
    hr: {
      backgroundColor: theme.colors.border,
      height: 1,
    },
    code_inline: {
      color: theme.colors.text,
      backgroundColor: theme.colors.panelSecondary,
      fontFamily: theme.font.code,
      borderRadius: 4,
      paddingHorizontal: 4,
      paddingVertical: 1,
    },
    code_block: {
      backgroundColor: theme.colors.panelSecondary,
      borderWidth: 1,
      borderColor: theme.colors.border,
      borderRadius: 6,
      padding: 0,
      marginBottom: 10,
      overflow: 'hidden' as const,
    },
    fence: {
      backgroundColor: theme.colors.panelSecondary,
      borderWidth: 1,
      borderColor: theme.colors.border,
      borderRadius: 6,
      padding: 0,
      marginBottom: 10,
      overflow: 'hidden' as const,
    },
    blockquote: {
      borderLeftColor: theme.colors.border,
      borderLeftWidth: 3,
      paddingLeft: 10,
      color: theme.colors.textMuted,
      marginBottom: 10,
    },
    link: {color: theme.colors.accent},
    table: {
      borderWidth: 1,
      borderColor: theme.colors.border,
      marginBottom: 10,
    },
    thead: {
      backgroundColor: theme.colors.panelSecondary,
    },
    th: {
      borderColor: theme.colors.border,
      padding: 6,
      color: theme.colors.text,
    },
    td: {
      borderColor: theme.colors.border,
      padding: 6,
      color: theme.colors.text,
    },
  };

  return (
    <View style={[styles.wrap, {backgroundColor: theme.colors.markdownBackground}]}>
      <Markdown
        style={markdownStyle}
        rules={{
          fence: node => {
            const language = parseFenceLanguage(node as MarkdownNode);
            const code = extractFenceCode(node as MarkdownNode);
            return (
              <View key={(node as MarkdownNode).key} style={markdownStyle.fence}>
                <ScrollView horizontal showsHorizontalScrollIndicator={Platform.OS === 'web'}>
                  <SyntaxHighlighter
                    language={language}
                    style={syntaxStyle}
                    customStyle={codeContainerStyle}
                    highlighter="hljs"
                    wrapLongLines={false}
                    fontFamily={theme.font.code}
                    fontSize={13}>
                    {code}
                  </SyntaxHighlighter>
                </ScrollView>
              </View>
            );
          },
          code_block: node => {
            const code = extractFenceCode(node as MarkdownNode);
            return (
              <View key={(node as MarkdownNode).key} style={markdownStyle.code_block}>
                <ScrollView horizontal showsHorizontalScrollIndicator={Platform.OS === 'web'}>
                  <SyntaxHighlighter
                    language="plaintext"
                    style={syntaxStyle}
                    customStyle={codeContainerStyle}
                    highlighter="hljs"
                    wrapLongLines={false}
                    fontFamily={theme.font.code}
                    fontSize={13}>
                    {code}
                  </SyntaxHighlighter>
                </ScrollView>
              </View>
            );
          },
          // Keep list bullets visible on web where some markdown styles collapse marker width.
          bullet_list_icon: (_node, _children, _parent, _styles) => (
            <Text style={{color: theme.colors.text}}>{'\u2022'}</Text>
          ),
        }}>
        {content || ''}
      </Markdown>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    minHeight: 160,
    borderRadius: 6,
    padding: 10,
  },
});
