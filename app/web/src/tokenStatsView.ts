import type { RegistryTokenProviderAccount } from './types/registry';

export type TokenProviderAccountView = RegistryTokenProviderAccount & {
  hubId: string;
  projectId: string;
  providerId: string;
  providerName: string;
};

export type TokenProviderSectionView = {
  id: string;
  name: string;
  accounts: TokenProviderAccountView[];
};

export type TokenStatCardView = {
  id: string;
  accountName: string;
  agentTag: string;
  hubTags: string[];
  message?: string;
  secondaryLine: string;
  tertiaryLine: string;
};

const normalizeTokenTagLabel = (value: string | undefined, fallback: string): string => {
  const normalized = (value || '').trim();
  return normalized || fallback;
};

const normalizeAccountIdentity = (value: string | undefined): string => {
  const normalized = (value || '').trim();
  if (!normalized || /^current(?:\s+account)?$/i.test(normalized)) {
    return '';
  }
  return normalized.toLowerCase();
};

const resolveAccountName = (account: TokenProviderAccountView): string => {
  const accountNameCandidates = [
    (account.email || '').trim(),
    (account.displayName || '').trim(),
    (account.alias || '').trim(),
  ].filter(Boolean);
  return (
    accountNameCandidates.find(name => !/^current(?:\s+account)?$/i.test(name)) ||
    accountNameCandidates[0] ||
    '(unnamed)'
  );
};

const resolveAccountGroupKey = (
  provider: TokenProviderSectionView,
  account: TokenProviderAccountView,
  accountName: string,
): string => {
  const identity =
    normalizeAccountIdentity(account.email) ||
    normalizeAccountIdentity(account.id) ||
    normalizeAccountIdentity(account.alias) ||
    normalizeAccountIdentity(account.displayName) ||
    accountName.toLowerCase();
  return `${provider.id}:${identity}`;
};

const formatCodexUsageLine = (label: '5h Usage' | 'Week Usage', value?: string): string => {
  const normalized = (value || '').trim();
  return `${label}: ${normalized || '-'}`;
};

const formatCopilotRequestLine = (account: TokenProviderAccountView): string => {
  const usedKnown = typeof account.premiumRequestsUsed === 'number';
  const remainingKnown = typeof account.premiumRequestsRemaining === 'number';
  const used: number = usedKnown ? account.premiumRequestsUsed ?? 0 : 0;
  const remaining: number = remainingKnown ? account.premiumRequestsRemaining ?? 0 : 0;
  const usedText = usedKnown ? used.toLocaleString() : '-';
  if (!usedKnown || !remainingKnown) {
    return `Request Used: ${usedText} / - · -`;
  }
  const total = used + remaining;
  const percent = total > 0 ? `${((used / total) * 100).toFixed(1)}%` : '0.0%';
  return `Request Used: ${used.toLocaleString()} / ${total.toLocaleString()} · ${percent}`;
};

const resolveUsageLines = (
  provider: TokenProviderSectionView,
  account: TokenProviderAccountView,
): {secondaryLine: string; tertiaryLine: string} => {
  const usageTotal = (account.usage?.rows || []).reduce(
    (sum, row) => sum + (row.totalTokens || 0),
    0,
  );

  if (provider.id === 'codex') {
    return {
      secondaryLine: formatCodexUsageLine('5h Usage', account.fiveHourLimit),
      tertiaryLine: formatCodexUsageLine('Week Usage', account.weeklyLimit),
    };
  }
  if (provider.id === 'copilot') {
    return {
      secondaryLine: formatCopilotRequestLine(account),
      tertiaryLine: '',
    };
  }
  if (provider.id === 'deepseek') {
    return {
      secondaryLine: `Balance: ${(account.balance?.items || [])
        .map(item => `${item.currency}:${item.totalBalance}`)
        .join(' | ') || '-'}`,
      tertiaryLine: `Tokens: ${usageTotal.toLocaleString()}`,
    };
  }
  return {
    secondaryLine: '-',
    tertiaryLine: '',
  };
};

export const buildTokenStatCards = (providers: TokenProviderSectionView[]): TokenStatCardView[] => {
  const groupedCards = new Map<string, TokenStatCardView>();
  for (const provider of providers) {
    const agentTag = normalizeTokenTagLabel(
      provider.name || provider.id,
      (provider.id || 'unknown').toUpperCase(),
    );
    for (const account of provider.accounts) {
      const accountName = resolveAccountName(account);
      const groupKey = resolveAccountGroupKey(provider, account, accountName);
      const hubTag = normalizeTokenTagLabel(account.hubId, 'local');
      const {secondaryLine, tertiaryLine} = resolveUsageLines(provider, account);
      const existingCard = groupedCards.get(groupKey);
      if (existingCard) {
        if (!existingCard.hubTags.includes(hubTag)) {
          existingCard.hubTags.push(hubTag);
        }
        if (!existingCard.message && account.message) {
          existingCard.message = account.message;
        }
        if ((!existingCard.secondaryLine || existingCard.secondaryLine === '-') && secondaryLine !== '-') {
          existingCard.secondaryLine = secondaryLine;
        }
        if (!existingCard.tertiaryLine && tertiaryLine) {
          existingCard.tertiaryLine = tertiaryLine;
        }
        continue;
      }
      groupedCards.set(groupKey, {
        id: groupKey,
        accountName,
        agentTag,
        hubTags: [hubTag],
        message: account.message,
        secondaryLine,
        tertiaryLine,
      });
    }
  }
  return Array.from(groupedCards.values()).sort((left, right) => {
    const agentDiff = left.agentTag.localeCompare(right.agentTag);
    if (agentDiff !== 0) return agentDiff;
    return left.accountName.localeCompare(right.accountName);
  });
};
