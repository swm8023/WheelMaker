import { buildTokenStatCards, type TokenProviderSectionView } from '../web/src/tokenStatsView';

describe('web token stats view', () => {
  test('aggregates the same provider account across hubs and keeps hub tags', () => {
    const providers: TokenProviderSectionView[] = [
      {
        id: 'codex',
        name: 'Codex',
        accounts: [
          {
            id: 'current',
            alias: 'Current Account',
            displayName: 'Current Account',
            email: 'dev@example.com',
            source: 'local',
            status: 'ok',
            hubId: 'local',
            projectId: 'local:repo',
            providerId: 'codex',
            providerName: 'Codex',
            fiveHourLimit: '4%',
            weeklyLimit: '12%',
            balance: {items: []},
            usage: {rows: []},
            usageUnavailable: false,
          },
          {
            id: 'current',
            alias: 'Current Account',
            displayName: 'Current Account',
            email: 'dev@example.com',
            source: 'ks',
            status: 'ok',
            hubId: 'ks-hub',
            projectId: 'ks-hub:repo',
            providerId: 'codex',
            providerName: 'Codex',
            fiveHourLimit: '4%',
            weeklyLimit: '12%',
            balance: {items: []},
            usage: {rows: []},
            usageUnavailable: false,
          },
        ],
      },
    ];

    const cards = buildTokenStatCards(providers);

    expect(cards).toHaveLength(1);
    expect(cards[0]).toMatchObject({
      accountName: 'dev@example.com',
      agentTag: 'Codex',
      hubTags: ['local', 'ks-hub'],
      secondaryLine: '5h Usage: 4%',
      tertiaryLine: 'Week Usage: 12%',
    });
  });
});
