import {describe, expect, it} from '@jest/globals';
import {buildPrismCodeBlockConfig} from './codeBlockConfig';

describe('buildPrismCodeBlockConfig', () => {
  it('does not override line layout styles owned by react-syntax-highlighter', () => {
    const config = buildPrismCodeBlockConfig({
      tabSize: 4,
      highlightLine: 3,
    });

    expect(config.codeTagProps.style).not.toHaveProperty('whiteSpace');

    const highlightedLine = config.lineProps(3);
    expect(highlightedLine.style).toEqual({
      background: 'rgba(0, 122, 204, 0.24)',
    });

    const normalLine = config.lineProps(2);
    expect(normalLine.style).toEqual({
      background: 'transparent',
    });
  });
});
