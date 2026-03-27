import React, {useMemo} from 'react';
import {Platform, ScrollView, StyleSheet, Text, View} from 'react-native';

import type {AppTheme} from '../theme';

const Prism = require('prismjs');
require('prismjs/components/prism-markup');
require('prismjs/components/prism-clike');
require('prismjs/components/prism-javascript');
require('prismjs/components/prism-jsx');
require('prismjs/components/prism-typescript');
require('prismjs/components/prism-tsx');
require('prismjs/components/prism-json');
require('prismjs/components/prism-markdown');
require('prismjs/components/prism-yaml');
require('prismjs/components/prism-css');
require('prismjs/components/prism-scss');
require('prismjs/components/prism-go');
require('prismjs/components/prism-rust');
require('prismjs/components/prism-python');
require('prismjs/components/prism-java');
require('prismjs/components/prism-kotlin');
require('prismjs/components/prism-c');
require('prismjs/components/prism-cpp');
require('prismjs/components/prism-csharp');
require('prismjs/components/prism-php');
require('prismjs/components/prism-bash');
require('prismjs/components/prism-powershell');
require('prismjs/components/prism-sql');
require('prismjs/components/prism-swift');
require('prismjs/components/prism-ruby');
require('prismjs/components/prism-docker');
require('prismjs/components/prism-ini');
require('prismjs/components/prism-makefile');
require('prismjs/components/prism-dart');
require('prismjs/components/prism-lua');

type PrismToken = string | PrismTokenObj | PrismToken[];

type PrismTokenObj = {
  type: string;
  alias?: string | string[];
  content: PrismToken;
};

type PrismCodeBlockProps = {
  language: string;
  code: string;
  theme: AppTheme;
  wrapLines?: boolean;
  showLineNumbers?: boolean;
  backgroundColor?: string;
};

const DARK_COLORS: Record<string, string> = {
  comment: '#6a9955',
  prolog: '#6a9955',
  doctype: '#6a9955',
  cdata: '#6a9955',
  punctuation: '#d4d4d4',
  namespace: '#d4d4d4',
  property: '#9cdcfe',
  tag: '#569cd6',
  boolean: '#569cd6',
  number: '#b5cea8',
  constant: '#4fc1ff',
  symbol: '#b5cea8',
  deleted: '#ce9178',
  selector: '#d7ba7d',
  attrName: '#9cdcfe',
  string: '#ce9178',
  char: '#ce9178',
  builtin: '#4ec9b0',
  inserted: '#b5cea8',
  operator: '#d4d4d4',
  entity: '#569cd6',
  url: '#ce9178',
  variable: '#9cdcfe',
  atrule: '#c586c0',
  attrValue: '#ce9178',
  function: '#dcdcaa',
  className: '#4ec9b0',
  keyword: '#c586c0',
  regex: '#d16969',
  important: '#c586c0',
  italic: '#d4d4d4',
  bold: '#d4d4d4',
};

const LIGHT_COLORS: Record<string, string> = {
  comment: '#008000',
  prolog: '#008000',
  doctype: '#008000',
  cdata: '#008000',
  punctuation: '#1f2328',
  namespace: '#1f2328',
  property: '#005cc5',
  tag: '#0000ff',
  boolean: '#0000ff',
  number: '#098658',
  constant: '#005cc5',
  symbol: '#098658',
  deleted: '#a31515',
  selector: '#800000',
  attrName: '#e36209',
  string: '#a31515',
  char: '#a31515',
  builtin: '#795e26',
  inserted: '#098658',
  operator: '#1f2328',
  entity: '#0000ff',
  url: '#a31515',
  variable: '#001080',
  atrule: '#af00db',
  attrValue: '#a31515',
  function: '#795e26',
  className: '#267f99',
  keyword: '#0000ff',
  regex: '#811f3f',
  important: '#af00db',
  italic: '#1f2328',
  bold: '#1f2328',
};

function normalizeLanguage(language: string): string {
  const value = (language || 'plaintext').toLowerCase();
  if (value === 'html' || value === 'xml') return 'markup';
  if (value === 'shell' || value === 'sh' || value === 'zsh') return 'bash';
  if (value === 'dockerfile') return 'docker';
  if (value === 'plaintext' || value === 'text') return 'none';
  return value;
}

function tokenTypes(token: PrismTokenObj): string[] {
  const aliasList = Array.isArray(token.alias)
    ? token.alias
    : token.alias
      ? [token.alias]
      : [];
  return [token.type, ...aliasList].map(item => String(item));
}

function tokenColor(token: PrismTokenObj, theme: AppTheme): string | undefined {
  const colors = theme.mode === 'dark' ? DARK_COLORS : LIGHT_COLORS;
  const types = tokenTypes(token);
  for (let i = 0; i < types.length; i += 1) {
    const mapped = colors[types[i]];
    if (mapped) return mapped;
  }
  return undefined;
}

function renderToken(token: PrismToken, key: string, theme: AppTheme): React.ReactNode {
  if (typeof token === 'string') {
    return <Text key={key}>{token}</Text>;
  }

  if (Array.isArray(token)) {
    return token.map((item, index) => renderToken(item, `${key}-${index}`, theme));
  }

  const content = renderToken(token.content, `${key}-c`, theme);
  const color = tokenColor(token, theme);

  return (
    <Text key={key} style={color ? {color} : undefined}>
      {content}
    </Text>
  );
}

function highlightLine(line: string, normalizedLanguage: string): PrismToken[] {
  if (normalizedLanguage === 'none') {
    return [line];
  }
  const grammar = Prism.languages[normalizedLanguage];
  if (!grammar) {
    return [line];
  }
  return Prism.tokenize(line, grammar) as PrismToken[];
}

export function PrismCodeBlock({
  language,
  code,
  theme,
  wrapLines = true,
  showLineNumbers = true,
  backgroundColor,
}: PrismCodeBlockProps) {
  const normalizedLanguage = normalizeLanguage(language);
  const lines = useMemo(() => {
    const input = code ?? '';
    return input.split('\n');
  }, [code]);
  const lineCountDigits = String(Math.max(lines.length, 1)).length;
  const lineNumberWidth = Math.max(30, lineCountDigits * 9 + 12);

  const lineRows = (
    <View style={styles.codeArea}>
      {lines.map((line, index) => {
        const highlighted = highlightLine(line, normalizedLanguage);
        return (
          <View key={`line-${index}`} style={styles.lineRow}>
            {showLineNumbers ? (
              <View style={[styles.lineNumberCol, {width: lineNumberWidth}]}>
                <Text
                  style={[
                    styles.lineNumberText,
                    {
                      color: theme.colors.textMuted,
                      fontFamily: theme.font.code,
                      borderRightColor: theme.colors.border,
                    },
                  ]}>
                  {index + 1}
                </Text>
              </View>
            ) : null}
            <Text
              style={[
                styles.lineText,
                {
                  color: theme.colors.text,
                  fontFamily: theme.font.code,
                },
                wrapLines ? styles.wrapLine : styles.noWrapLine,
              ]}>
              {line.length === 0 ? '\u200B' : highlighted.map((token, tokenIndex) => renderToken(token, `t-${index}-${tokenIndex}`, theme))}
            </Text>
          </View>
        );
      })}
    </View>
  );

  return (
    <View style={[styles.wrap, {backgroundColor: backgroundColor ?? theme.colors.codeBackground}]}>
      {wrapLines ? (
        lineRows
      ) : (
        <ScrollView
          horizontal
          style={styles.noWrapScroller}
          showsHorizontalScrollIndicator={Platform.OS === 'web'}
          contentContainerStyle={styles.scrollContent}>
          {lineRows}
        </ScrollView>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: {
    flex: 1,
    width: '100%',
    minHeight: 0,
    borderRadius: 6,
    overflow: 'hidden',
  },
  noWrapScroller: {
    flex: 1,
    minHeight: 0,
    width: '100%',
  },
  scrollContent: {
    minWidth: '100%',
    flexGrow: 1,
  },
  codeArea: {
    minWidth: '100%',
    flexGrow: 1,
    alignSelf: 'stretch',
  },
  lineRow: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    minHeight: 20,
  },
  lineNumberCol: {
    alignItems: 'flex-end',
    backgroundColor: 'transparent',
  },
  lineNumberText: {
    width: '100%',
    textAlign: 'right',
    fontSize: 13,
    lineHeight: 20,
    paddingRight: 8,
    borderRightWidth: 1,
  },
  lineText: {
    flex: 1,
    fontSize: 13,
    lineHeight: 20,
    paddingLeft: 8,
    paddingRight: 8,
  },
  wrapLine: {
    flexShrink: 1,
    flexGrow: 1,
  },
  noWrapLine: {
    minWidth: '100%',
    flexShrink: 0,
    flexGrow: 0,
  },
});
