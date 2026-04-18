import fs from 'fs';
import path from 'path';

describe('web markdown preview mode', () => {
  test('shows markdown preview toggle under search and wires markdown+mermaid+latex pipeline', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain("import ReactMarkdown from 'react-markdown';");
    expect(mainTsx).toContain("import remarkGfm from 'remark-gfm';");
    expect(mainTsx).toContain("import remarkMath from 'remark-math';");
    expect(mainTsx).toContain("import rehypeKatex from 'rehype-katex';");
    expect(mainTsx).toContain("import mermaid from 'mermaid';");
    expect(mainTsx).toContain("import 'katex/dist/katex.min.css';");

    expect(mainTsx).toContain('function isMarkdownPath(path: string): boolean {');
    expect(mainTsx).toContain('const selectedFileIsMarkdown = isMarkdownPath(selectedFile);');
    expect(mainTsx).toContain('const [markdownPreviewEnabled, setMarkdownPreviewEnabled] = useState(false);');
    expect(mainTsx).toContain('if (!isMarkdownPath(selectedFile)) {');
    expect(mainTsx).toContain('setMarkdownPreviewEnabled(false);');

    expect(mainTsx).toContain('aria-label="Toggle markdown preview"');
    expect(mainTsx).toContain('className={`view-tool markdown-preview-toggle ${');
    expect(mainTsx).toContain('{selectedFileIsMarkdown ? (');

    const searchIndex = mainTsx.indexOf('aria-label="Toggle search"');
    const previewIndex = mainTsx.indexOf('aria-label="Toggle markdown preview"');
    expect(searchIndex).toBeGreaterThan(-1);
    expect(previewIndex).toBeGreaterThan(searchIndex);

    expect(mainTsx).toContain('<MarkdownPreview');
    expect(mainTsx).toContain('remarkPlugins={[remarkGfm, remarkMath]}');
    expect(mainTsx).toContain('rehypePlugins={[rehypeKatex]}');
    expect(mainTsx).toContain('if (language === \"mermaid\") {');
    expect(mainTsx).toContain('<MermaidBlock content={codeText} themeMode={themeMode} />');

    expect(stylesCss).toContain('.markdown-preview {');
    expect(stylesCss).toContain('.markdown-preview-toggle {');
    expect(stylesCss).toContain('.mermaid-block {');
    expect(stylesCss).toContain('.mermaid-error {');
  });
});
