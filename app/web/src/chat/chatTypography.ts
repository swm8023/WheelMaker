export type ChatFontId = 'system' | 'ibm-plex' | 'serif';

export type ChatFontOption = {
  id: ChatFontId;
  label: string;
  fontFamily: string;
};

export const DEFAULT_CHAT_FONT: ChatFontId = 'ibm-plex';

export const CHAT_FONT_OPTIONS: ChatFontOption[] = [
  {
    id: 'ibm-plex',
    label: 'IBM Plex Sans',
    fontFamily: "'IBM Plex Sans', 'Noto Sans', sans-serif",
  },
  {
    id: 'system',
    label: 'System Sans',
    fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', 'Microsoft YaHei UI', 'PingFang SC', 'Noto Sans CJK SC', 'Noto Sans', sans-serif",
  },
  {
    id: 'serif',
    label: 'Serif',
    fontFamily: "Georgia, 'Times New Roman', 'Noto Serif CJK SC', serif",
  },
];

export function isChatFontId(value: string): value is ChatFontId {
  return CHAT_FONT_OPTIONS.some(option => option.id === value);
}

export function resolveChatFontFamily(chatFont: ChatFontId): string {
  return CHAT_FONT_OPTIONS.find(option => option.id === chatFont)?.fontFamily ?? CHAT_FONT_OPTIONS[0].fontFamily;
}
