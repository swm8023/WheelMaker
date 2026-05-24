import fs from 'fs';
import path from 'path';

describe('web markdown preview mode', () => {
  test('shows markdown preview toggle before wrap and wires markdown+mermaid+latex pipeline', () => {
    const projectRoot = path.join(__dirname, '..');
    const mainTsx = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const stylesCss = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(mainTsx).toContain("import ReactMarkdown");
    expect(mainTsx).toContain("from 'react-markdown';");
    expect(mainTsx).toContain("import remarkGfm from 'remark-gfm';");
    expect(mainTsx).not.toContain("import remarkMath from 'remark-math';");
    expect(mainTsx).not.toContain("import rehypeKatex from 'rehype-katex';");
    expect(mainTsx).not.toContain("import mermaid from 'mermaid';");
    expect(mainTsx).not.toContain("import 'katex/dist/katex.min.css';");
    expect(mainTsx).toContain("import('remark-math')");
    expect(mainTsx).toContain("import('rehype-katex')");
    expect(mainTsx).toContain("import('katex/dist/katex.min.css')");
    expect(mainTsx).toContain("import('mermaid')");
    expect(mainTsx).toContain('function markdownNeedsMath(content: string): boolean {');
    expect(mainTsx).toContain('function useMarkdownCapabilityPlugins(content: string): MarkdownCapabilityPlugins {');

    expect(mainTsx).toContain('function isMarkdownPath(path: string): boolean {');
    expect(mainTsx).toContain('const selectedFileIsMarkdown = isMarkdownPath(selectedFile);');
    expect(mainTsx).toContain('const [markdownPreviewEnabled, setMarkdownPreviewEnabled] = useState(false);');
    expect(mainTsx).toContain('setMarkdownPreviewEnabled(isMarkdownPath(selectedFile));');

    expect(mainTsx).toContain('aria-label="Toggle markdown preview"');
    expect(mainTsx).toContain('className={`view-tool markdown-preview-toggle ${');
    expect(mainTsx).toContain('<span className="markdown-preview-toggle-text">MD</span>');
    expect(mainTsx).toContain('{selectedFileIsMarkdown ? (');

    const previewIndex = mainTsx.indexOf('aria-label="Toggle markdown preview"');
    const wrapIndex = mainTsx.indexOf('aria-label="Toggle wrap line"');
    expect(previewIndex).toBeGreaterThan(-1);
    expect(wrapIndex).toBeGreaterThan(previewIndex);

    expect(mainTsx).toContain('<MarkdownPreview');
    expect(mainTsx).toContain('remarkPlugins={markdownCapabilities.remarkPlugins}');
    expect(mainTsx).toContain('rehypePlugins={markdownCapabilities.rehypePlugins}');
    expect(mainTsx).toContain("data-markdown-export-pending={markdownCapabilities.pending ? 'true' : undefined}");
    expect(mainTsx).toContain('if (language === "mermaid") {');
    expect(mainTsx).toContain('<MermaidBlock content={codeText} themeMode={themeMode} />');

    expect(stylesCss).toContain('.markdown-preview {');
    expect(stylesCss).toContain('.markdown-preview-toggle {');
    expect(stylesCss).toContain('.mermaid-block {');
    expect(stylesCss).toContain('.mermaid-error {');
  });
});
