import { buildTokenStatCards, type TokenProviderSectionView } from '../web/src/tokenStatsView';
import fs from 'fs';
import path from 'path';

const mainTsx = fs.readFileSync(path.join(__dirname, '../web/src/main.tsx'), 'utf8');

describe('web token stats view', () => {
  test('refreshes token stats from hub snapshot instead of online projects', () => {
    const refreshTokenStatsStart = mainTsx.indexOf('const refreshTokenStats = useCallback(async () => {');
    const refreshTokenStatsEnd = mainTsx.indexOf('const agentPackageActionKey', refreshTokenStatsStart);
    const refreshTokenStatsBlock = mainTsx.slice(refreshTokenStatsStart, refreshTokenStatsEnd);

    expect(refreshTokenStatsBlock).toContain('const snapshot = await service.listProjectSnapshot();');
    expect(refreshTokenStatsBlock).toContain('const hubIds = deriveRegistryHubIds(snapshot.hubs);');
    expect(refreshTokenStatsBlock).toContain('service.scanTokenStats(hubId)');
    expect(refreshTokenStatsBlock).not.toContain('onlineByHub');
    expect(refreshTokenStatsBlock).not.toContain('scanTokenStats(project.projectId)');
  });

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
