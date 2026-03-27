import React from 'react';

import type {AppTheme} from '../theme';
import {languageFromPath} from '../utils/codeLanguage';
import {PrismCodeBlock} from './PrismCodeBlock';

type PrismCodeViewProps = {
  path: string;
  code: string;
  theme: AppTheme;
  wrapLines?: boolean;
  showLineNumbers?: boolean;
};

export function PrismCodeView({
  path,
  code,
  theme,
  wrapLines = true,
  showLineNumbers = true,
}: PrismCodeViewProps) {
  return (
    <PrismCodeBlock
      language={languageFromPath(path)}
      code={code}
      theme={theme}
      wrapLines={wrapLines}
      showLineNumbers={showLineNumbers}
    />
  );
}
