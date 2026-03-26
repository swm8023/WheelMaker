import React from 'react';
import {StyleSheet, View} from 'react-native';
import SyntaxHighlighter from 'react-native-syntax-highlighter';
import {vs2015, vs} from 'react-syntax-highlighter/styles/hljs';

import type {AppTheme} from '../theme';
import {languageFromPath} from '../utils/codeLanguage';

type CodeViewProps = {
  path: string;
  code: string;
  theme: AppTheme;
};

export function CodeView({path, code, theme}: CodeViewProps) {
  const language = languageFromPath(path);
  const style = theme.mode === 'dark' ? vs2015 : vs;

  return (
    <View style={[styles.wrap, {backgroundColor: theme.colors.codeBackground}]}>
      <SyntaxHighlighter
        language={language}
        style={style}
        highlighter="hljs"
        fontFamily={theme.font.code}
        fontSize={13}>
        {code || ''}
      </SyntaxHighlighter>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    minHeight: 160,
    borderRadius: 6,
    overflow: 'hidden',
  },
});
