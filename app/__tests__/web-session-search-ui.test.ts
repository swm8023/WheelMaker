import fs from 'fs';
import path from 'path';

describe('web session search UI wiring', () => {
  test('keeps search protocol wiring with prompt turn navigation', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(main).toContain('searchResultsByProjectId');
    expect(main).toContain('startSessionSearch');
    expect(main).toContain('querySessionSearch');
    expect(main).toContain('cancelSessionSearch');
    expect(main).toContain('scrollToTurnIndex');
    expect(main).toContain('sessionSearchTargetTurn');
    expect(styles).toContain('.session-search-control');
    expect(styles).toContain('.session-search-result-meta');
    expect(styles).toContain('.chat-turn-search-highlight');
  });

  test('moves session search controls into desktop and mobile chat headers', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(main).toContain('const renderChatHeaderSearchControls = (mobile: boolean) =>');
    expect(main).toContain('const renderSessionSearchStatusLine = () =>');
    expect(main).toContain('const sessionSearchHeaderExpanded = sessionSearchOpen || sessionSearchActive;');
    expect(main).toContain("const chatSidebarTitleSearchOpen = tab === 'chat' && !sidebarSettingsOpen && sessionSearchHeaderExpanded;");
    expect(main).toContain('className={`sidebar-title-row${chatSidebarTitleSearchOpen ? \' search-open\' : \'\'}`}');

    const wideHeaderStart = main.indexOf('className={`sidebar-title-row${chatSidebarTitleSearchOpen ?');
    const wideHeaderEnd = main.indexOf('<div className="sidebar-scroll">', wideHeaderStart);
    expect(wideHeaderStart).toBeGreaterThanOrEqual(0);
    expect(wideHeaderEnd).toBeGreaterThan(wideHeaderStart);
    const wideHeader = main.slice(wideHeaderStart, wideHeaderEnd);
    expect(wideHeader).toContain('renderChatHeaderSearchControls(false)');
    expect(wideHeader).toContain('renderChatHubSummary()');
    expect(wideHeader.indexOf('renderChatHubSummary()')).toBeLessThan(wideHeader.lastIndexOf('renderChatHeaderSearchControls(false)'));

    const wideNavStart = main.indexOf('const renderWideProjectSessionNav = () =>');
    const wideNavEnd = main.indexOf('const renderCodePane = (', wideNavStart);
    expect(wideNavStart).toBeGreaterThanOrEqual(0);
    expect(wideNavEnd).toBeGreaterThan(wideNavStart);
    const wideNav = main.slice(wideNavStart, wideNavEnd);
    expect(wideNav).not.toContain('renderSessionSearchControls()');

    const mobileHeaderStart = main.indexOf('className={`mobile-chat-drawer-header${sessionSearchHeaderExpanded ?');
    const mobileHeaderEnd = main.indexOf('{sessionSearchActive ? renderSessionSearchResults(true)', mobileHeaderStart);
    expect(mobileHeaderStart).toBeGreaterThanOrEqual(0);
    expect(mobileHeaderEnd).toBeGreaterThan(mobileHeaderStart);
    const mobileHeader = main.slice(mobileHeaderStart, mobileHeaderEnd);
    expect(mobileHeader).toContain('renderChatHeaderSearchControls(true)');
    expect(mobileHeader).toContain('renderChatHubSummary()');
    expect(mobileHeader.indexOf('renderChatHeaderSearchControls(true)')).toBeLessThan(mobileHeader.indexOf('renderChatHubSummary()'));

    expect(styles).toContain('.chat-header-search-control');
    expect(styles).toContain('.chat-header-search-control.open');
    expect(styles).toContain('.chat-header-search-status');
    expect(styles).toContain('.sidebar-title-row.search-open');
    expect(styles).toContain('.mobile-chat-drawer-header.search-open');
  });

  test('renders full Hub labels and aggregate header search status text', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    expect(main).toContain("const chatHubSummaryLabel = `${hubCount} ${hubCount === 1 ? 'Hub' : 'Hubs'}`;");
    expect(main).toContain('const sessionSearchProjectDoneCount = useMemo(');
    expect(main).toContain("`Searching ${sessionSearchProjectDoneCount}/${sortedProjectItems.length} projects`");
    expect(main).toContain('parts.push(`${sessionSearchErrorCount} error${sessionSearchErrorCount === 1 ? \'\' : \'s\'}`);');
    expect(main).toContain('sessionSearchStatusParts.join(\' · \')');
    expect(main).not.toContain('className="chat-hub-summary-count"');

    const hubButtonBlock = styles.match(/\.chat-hub-summary-button \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(hubButtonBlock).toContain('white-space: nowrap;');
    expect(styles).not.toContain('.chat-hub-summary-label {\n  display: none;');
    expect(styles).not.toContain('.chat-hub-summary-count {');
  });
});
