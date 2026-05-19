type ChatSessionTitleFacts = {
  first?: unknown;
  last?: unknown;
};

function normalizedText(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

export function resolveChatSessionTitle(rawTitle: string, useLatestPromptTitle: boolean): string {
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
    const preferred = useLatestPromptTitle ? last : first;
    return preferred || first || last || legacyTitle;
  } catch {
    return legacyTitle;
  }
}
