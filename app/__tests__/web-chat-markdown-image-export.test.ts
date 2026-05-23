import {
  buildPromptMarkdownImageFileName,
  waitForMarkdownExportReady,
  waitForMarkdownExportImages,
} from '../web/src/chatMarkdownImageExport';

describe('web chat markdown image export', () => {
  test('builds a stable png filename for prompt-done exports', () => {
    expect(
      buildPromptMarkdownImageFileName(7, new Date('2026-05-23T10:11:12.345Z')),
    ).toBe('wheelmaker-response-turn-7-2026-05-23T10-11-12-345Z.png');
  });

  test('waits for pending markdown images before capture', async () => {
    const loadedImage = {
      complete: true,
      naturalWidth: 120,
    };
    const pendingImage = {
      complete: false,
      naturalWidth: 0,
      decode: jest.fn(() => Promise.resolve()),
    };
    const root = {
      querySelectorAll: jest.fn(() => [loadedImage, pendingImage]),
    };

    await waitForMarkdownExportImages(root, 250);

    expect(root.querySelectorAll).toHaveBeenCalledWith('img');
    expect(pendingImage.decode).toHaveBeenCalledTimes(1);
  });

  test('waits for async markdown renderers to leave pending state', async () => {
    let pendingChecks = 0;
    const root = {
      innerHTML: '<div>stable</div>',
      querySelectorAll: jest.fn((selector: string) => {
        if (selector === '[data-markdown-export-pending="true"]') {
          pendingChecks += 1;
          return pendingChecks === 1 ? [{}] : [];
        }
        if (selector === 'img') {
          return [];
        }
        return [];
      }),
    };

    await waitForMarkdownExportReady(root, {
      domQuietMs: 1,
      domTimeoutMs: 100,
      rendererTimeoutMs: 250,
    });

    expect(root.querySelectorAll).toHaveBeenCalledWith('[data-markdown-export-pending="true"]');
    expect(pendingChecks).toBeGreaterThan(1);
  });
});
