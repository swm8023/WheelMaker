export type PromptCopyEntry = {
  kind: string;
  text: string;
};

export const buildPromptAgentMarkdown = (entries: PromptCopyEntry[]): string => {
  return entries
    .filter(entry => entry.kind === 'message')
    .map(entry => entry.text.trim())
    .filter(Boolean)
    .join('\n\n')
    .trim();
};
