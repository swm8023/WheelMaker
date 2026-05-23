export type MarkdownExportImageLike = {
  complete?: boolean;
  naturalWidth?: number;
  decode?: () => Promise<void>;
  addEventListener?: (
    type: 'load' | 'error',
    listener: () => void,
    options?: { once?: boolean },
  ) => void;
  removeEventListener?: (
    type: 'load' | 'error',
    listener: () => void,
  ) => void;
};

export type MarkdownExportRootLike = {
  innerHTML?: string;
  querySelectorAll: (
    selector: string,
  ) => ArrayLike<unknown> | Iterable<unknown>;
};

type MarkdownExportReadyOptions = {
  domQuietMs?: number;
  domTimeoutMs?: number;
  rendererTimeoutMs?: number;
  imageTimeoutMs?: number;
  fontTimeoutMs?: number;
};

type MarkdownElementPngOptions = MarkdownExportReadyOptions & {
  backgroundColor?: string;
  pixelRatio?: number;
};

export function buildPromptMarkdownImageFileName(
  doneTurnIndex: number,
  now = new Date(),
): string {
  const safeTurnIndex = Math.max(0, Math.trunc(doneTurnIndex || 0));
  const timestamp = now.toISOString().replace(/[:.]/g, '-');
  return `wheelmaker-response-turn-${safeTurnIndex}-${timestamp}.png`;
}

function imageIsSettled(image: MarkdownExportImageLike): boolean {
  return image.complete === true;
}

function waitForImage(image: MarkdownExportImageLike): Promise<void> {
  if (imageIsSettled(image)) {
    return Promise.resolve();
  }
  if (typeof image.decode === 'function') {
    return image.decode().catch(() => undefined);
  }
  if (typeof image.addEventListener !== 'function') {
    return Promise.resolve();
  }
  return new Promise(resolve => {
    const finish = () => {
      image.removeEventListener?.('load', finish);
      image.removeEventListener?.('error', finish);
      resolve();
    };
    image.addEventListener?.('load', finish, { once: true });
    image.addEventListener?.('error', finish, { once: true });
  });
}

async function withTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
): Promise<T | undefined> {
  if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) {
    return promise;
  }
  let timer: ReturnType<typeof setTimeout> | null = null;
  try {
    return await Promise.race([
      promise,
      new Promise<undefined>(resolve => {
        timer = setTimeout(() => resolve(undefined), timeoutMs);
      }),
    ]);
  } finally {
    if (timer) {
      clearTimeout(timer);
    }
  }
}

export async function waitForMarkdownExportImages(
  root: MarkdownExportRootLike,
  timeoutMs = 8000,
): Promise<void> {
  const images = Array.from(root.querySelectorAll('img') as Iterable<MarkdownExportImageLike>);
  const pending = images.filter(image => !imageIsSettled(image));
  if (pending.length === 0) {
    return;
  }
  await withTimeout(
    Promise.all(pending.map(waitForImage)).then(() => undefined),
    timeoutMs,
  );
}

async function waitForMarkdownExportRenderers(
  root: MarkdownExportRootLike,
  timeoutMs = 8000,
): Promise<void> {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const pending = Array.from(root.querySelectorAll('[data-markdown-export-pending="true"]'));
    if (pending.length === 0) {
      return;
    }
    await new Promise(resolve => setTimeout(resolve, 50));
  }
}

async function waitForMarkdownExportDomQuiet(
  root: { innerHTML?: string },
  quietMs = 180,
  timeoutMs = 5000,
): Promise<void> {
  const startedAt = Date.now();
  let latestHtml = root.innerHTML ?? '';
  let quietStartedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    await new Promise(resolve => setTimeout(resolve, 50));
    const currentHtml = root.innerHTML ?? '';
    if (currentHtml !== latestHtml) {
      latestHtml = currentHtml;
      quietStartedAt = Date.now();
      continue;
    }
    if (Date.now() - quietStartedAt >= quietMs) {
      return;
    }
  }
}

async function waitForDocumentFonts(timeoutMs = 3000): Promise<void> {
  const fontsReady = typeof document !== 'undefined' ? document.fonts?.ready : null;
  if (!fontsReady) {
    return;
  }
  await withTimeout(fontsReady.then(() => undefined), timeoutMs);
}

export async function waitForMarkdownExportReady(
  root: MarkdownExportRootLike,
  options: MarkdownExportReadyOptions = {},
): Promise<void> {
  await waitForMarkdownExportRenderers(root, options.rendererTimeoutMs);
  await waitForMarkdownExportDomQuiet(
    root,
    options.domQuietMs,
    options.domTimeoutMs,
  );
  await waitForDocumentFonts(options.fontTimeoutMs);
  await waitForMarkdownExportImages(root, options.imageTimeoutMs);
}

export async function renderMarkdownElementToPngBlob(
  element: HTMLElement,
  options: MarkdownElementPngOptions = {},
): Promise<Blob> {
  await waitForMarkdownExportReady(element, options);
  const { toBlob } = await import('html-to-image');
  const pixelRatio = options.pixelRatio ?? Math.min(Math.max(window.devicePixelRatio || 1, 1), 2);
  const blob = await toBlob(element, {
    backgroundColor: options.backgroundColor || '#ffffff',
    cacheBust: true,
    includeQueryParams: true,
    pixelRatio,
  });
  if (!blob) {
    throw new Error('Image renderer returned an empty file.');
  }
  return blob;
}

export function downloadBlobAsFile(blob: Blob, fileName: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const link = document.createElement('a');
  try {
    link.href = objectUrl;
    link.download = fileName;
    link.click();
  } finally {
    URL.revokeObjectURL(objectUrl);
  }
}
