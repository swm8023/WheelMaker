import fs from 'fs';
import path from 'path';

describe('web git filename truncation policy', () => {
  test('keeps filename untruncated while parent path remains ellipsized', () => {
    const projectRoot = path.join(__dirname, '..');
    const stylesCss = fs.readFileSync(
      path.join(projectRoot, 'web', 'src', 'styles.css'),
      'utf8',
    );

    expect(stylesCss).toMatch(
      /\.git-file-name\s*\{[^}]*flex:\s*0 0 auto;[^}]*overflow:\s*visible;[^}]*text-overflow:\s*clip;/,
    );
    expect(stylesCss).toMatch(
      /\.git-file-path\s*\{[^}]*overflow:\s*hidden;[^}]*text-overflow:\s*ellipsis;/,
    );
  });
});