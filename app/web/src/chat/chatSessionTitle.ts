type ChatSessionTitleFacts = {
  first?: unknown;
  last?: unknown;
  manual?: unknown;
};

function normalizedText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

export function resolveChatSessionTitle(rawTitle: string): string {
  const legacyTitle = normalizedText(rawTitle);
  if (!legacyTitle.startsWith('{')) {
    return legacyTitle;
  }
  try {
    const parsed = JSON.parse(legacyTitle) as ChatSessionTitleFacts;
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return legacyTitle;
    }
    const first = normalizedText(parsed.first);
    const last = normalizedText(parsed.last);
    const manual = normalizedText(parsed.manual);
    return manual || first || last || legacyTitle;
  } catch {
    return legacyTitle;
  }
}
