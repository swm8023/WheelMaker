import React, {useMemo} from 'react';
import {ScrollView, StyleSheet, Text, View} from 'react-native';

import type {AppTheme} from '../theme';

type DiffRowType = 'meta' | 'hunk' | 'add' | 'del' | 'ctx';

type DiffRow = {
  type: DiffRowType;
  text: string;
  oldLine?: number;
  newLine?: number;
};

type InlineDiffViewProps = {
  diff: string;
  theme: AppTheme;
};

function parseUnifiedDiff(diff: string): DiffRow[] {
  const rows: DiffRow[] = [];
  let oldLine = 0;
  let newLine = 0;
  const lines = (diff || '').split('\n');
  for (const line of lines) {
    if (line.startsWith('@@')) {
      const match = line.match(/^@@\s-\s?(\d+)(?:,\d+)?\s\+(\d+)(?:,\d+)?\s@@/) ?? line.match(/^@@\s-(\d+)(?:,\d+)?\s\+(\d+)(?:,\d+)?\s@@/);
      if (match) {
        oldLine = Number.parseInt(match[1], 10);
        newLine = Number.parseInt(match[2], 10);
      }
      rows.push({type: 'hunk', text: line});
      continue;
    }
    if (
      line.startsWith('diff --git') ||
      line.startsWith('index ') ||
      line.startsWith('--- ') ||
      line.startsWith('+++ ') ||
      line.startsWith('\\ No newline at end of file')
    ) {
      rows.push({type: 'meta', text: line});
      continue;
    }
    if (line.startsWith('+') && !line.startsWith('+++')) {
      rows.push({type: 'add', text: line, newLine});
      newLine += 1;
      continue;
    }
    if (line.startsWith('-') && !line.startsWith('---')) {
      rows.push({type: 'del', text: line, oldLine});
      oldLine += 1;
      continue;
    }
    rows.push({type: 'ctx', text: line, oldLine, newLine});
    oldLine += 1;
    newLine += 1;
  }
  return rows;
}

function lineText(row: DiffRow): string {
  if (row.type === 'add' || row.type === 'del' || row.type === 'ctx') {
    return row.text.slice(1);
  }
  return row.text;
}

function lineSign(row: DiffRow): string {
  if (row.type === 'add') return '+';
  if (row.type === 'del') return '-';
  if (row.type === 'hunk') return '@';
  return ' ';
}

export function InlineDiffView({diff, theme}: InlineDiffViewProps) {
  const rows = useMemo(() => parseUnifiedDiff(diff), [diff]);
  const palette = theme.mode === 'dark'
    ? {
        addBg: '#12261e',
        delBg: '#2d1219',
        hunkBg: '#1f2633',
        metaBg: '#252526',
        gutter: '#858585',
        text: '#d4d4d4',
      }
    : {
        addBg: '#e8fff0',
        delBg: '#fff1f0',
        hunkBg: '#edf3ff',
        metaBg: '#f6f8fa',
        gutter: '#6e7781',
        text: '#1f2328',
      };

  return (
    <ScrollView horizontal style={styles.wrap} contentContainerStyle={styles.content}>
      <View style={styles.table}>
        {rows.map((row, index) => (
          <View
            key={`${row.type}-${index}`}
            style={[
              styles.row,
              row.type === 'add' && {backgroundColor: palette.addBg},
              row.type === 'del' && {backgroundColor: palette.delBg},
              row.type === 'hunk' && {backgroundColor: palette.hunkBg},
              row.type === 'meta' && {backgroundColor: palette.metaBg},
            ]}>
            <Text style={[styles.sign, {color: palette.gutter}]}>{lineSign(row)}</Text>
            <Text style={[styles.num, {color: palette.gutter}]}>
              {row.oldLine !== undefined ? String(row.oldLine) : ''}
            </Text>
            <Text style={[styles.num, {color: palette.gutter}]}>
              {row.newLine !== undefined ? String(row.newLine) : ''}
            </Text>
            <Text style={[styles.text, {color: palette.text}]}>{lineText(row)}</Text>
          </View>
        ))}
      </View>
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  wrap: {
    width: '100%',
  },
  content: {
    minWidth: '100%',
  },
  table: {
    minWidth: '100%',
  },
  row: {
    flexDirection: 'row',
    minHeight: 22,
    alignItems: 'center',
    paddingHorizontal: 8,
  },
  sign: {
    width: 14,
    fontSize: 12,
    lineHeight: 18,
    fontFamily: 'monospace',
  },
  num: {
    width: 42,
    fontSize: 12,
    lineHeight: 18,
    textAlign: 'right',
    marginRight: 8,
    fontFamily: 'monospace',
  },
  text: {
    fontSize: 13,
    lineHeight: 18,
    fontFamily: 'monospace',
  },
});
