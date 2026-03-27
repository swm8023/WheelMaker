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
  wrapLines?: boolean;
  showLineNumbers?: boolean;
};

export function CodeView({
  path,
  code,
  theme,
  wrapLines = true,
  showLineNumbers = true,
}: CodeViewProps) {
  const language = languageFromPath(path);
  const style = theme.mode === 'dark' ? vs2015 : vs;
  const webSyntaxStyle = {
    margin: 0,
    minHeight: '100%',
    overflowX: wrapLines ? 'visible' : 'auto',
    overflowY: 'visible',
    whiteSpace: wrapLines ? 'pre-wrap' : 'pre',
    wordBreak: wrapLines ? 'break-word' : 'normal',
  } as unknown as Record<string, unknown>;
  const webCodeTagStyle = {
    whiteSpace: wrapLines ? 'pre-wrap' : 'pre',
    wordBreak: wrapLines ? 'break-word' : 'normal',
  } as unknown as Record<string, unknown>;

  return (
    <View style={[styles.wrap, {backgroundColor: theme.colors.codeBackground}]}>
      <SyntaxHighlighter
        language={language}
        style={style}
        customStyle={webSyntaxStyle}
        codeTagProps={{style: webCodeTagStyle}}
        highlighter="hljs"
        wrapLongLines={wrapLines}
        showLineNumbers={showLineNumbers}
        fontFamily={theme.font.code}
        fontSize={13}>
        {code || ''}
      </SyntaxHighlighter>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    width: '100%',
    minHeight: 160,
    flexGrow: 1,
    borderRadius: 6,
    overflow: 'hidden',
  },
});

