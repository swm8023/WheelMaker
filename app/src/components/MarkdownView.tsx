import React from 'react';
import {StyleSheet, View} from 'react-native';
import Markdown from 'react-native-markdown-display';

import type {AppTheme} from '../theme';

type MarkdownViewProps = {
  content: string;
  theme: AppTheme;
};

export function MarkdownView({content, theme}: MarkdownViewProps) {
  return (
    <View style={[styles.wrap, {backgroundColor: theme.colors.markdownBackground}]}>
      <Markdown
        style={{
          body: {color: theme.colors.text, fontSize: 14},
          heading1: {color: theme.colors.text},
          heading2: {color: theme.colors.text},
          heading3: {color: theme.colors.text},
          code_inline: {
            color: theme.colors.text,
            backgroundColor: theme.colors.panelSecondary,
          },
          code_block: {
            color: theme.colors.text,
            backgroundColor: theme.colors.panelSecondary,
            fontFamily: theme.font.code,
          },
          fence: {
            color: theme.colors.text,
            backgroundColor: theme.colors.panelSecondary,
            fontFamily: theme.font.code,
          },
          blockquote: {
            borderLeftColor: theme.colors.border,
            color: theme.colors.textMuted,
          },
          link: {color: theme.colors.accent},
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
