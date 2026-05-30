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
    expect(styles).toContain('.chat-turn-search-highlight');
  });

  test('loads the matched prompt turn before applying search-result navigation', () => {
    const projectRoot = path.join(__dirname, '..');
    const main = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'main.tsx'), 'utf8');
    const clickStart = main.indexOf('const handleSessionSearchResultClick = async (');
    const clickEnd = main.indexOf('const renderSessionSearchHighlightedTitle = (', clickStart);
    expect(clickStart).toBeGreaterThanOrEqual(0);
    expect(clickEnd).toBeGreaterThan(clickStart);
    const clickBody = main.slice(clickStart, clickEnd);

    expect(clickBody).toContain("row.result.source === 'prompt'");
    expect(clickBody).toContain('targetTurnIndex: promptTargetTurnIndex');
    expect(main).toContain('const searchTargetTurnIsVisible = chatDisplayIndex.items.some(');
    expect(main).toContain('!searchTargetTurnIsVisible');

    const selectStart = main.indexOf('const selectProjectChatSession = async (');
    const selectEnd = main.indexOf('const selectWideProjectSession = async', selectStart);
    expect(selectStart).toBeGreaterThanOrEqual(0);
    expect(selectEnd).toBeGreaterThan(selectStart);
    const selectBody = main.slice(selectStart, selectEnd);
    expect(selectBody).toContain('targetTurnIndex?: number');
    expect(selectBody).toContain('forceFull: hasTargetTurnIndex');
    expect(selectBody).toContain('incremental: !hasTargetTurnIndex');
    expect(selectBody).toContain('revealTurnIndex: targetTurnIndex');
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
    expect(mobileHeader).toContain('renderChatHubSummary(true)');
    expect(mobileHeader.indexOf('renderChatHeaderSearchControls(true)')).toBeLessThan(mobileHeader.indexOf('renderChatHubSummary(true)'));

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
    expect(main).toContain("const chatHubProjectLabel = `${projectCount} ${projectCount === 1 ? 'Project' : 'Projects'}`;");
    expect(main).toContain('const sessionSearchProjectDoneCount = useMemo(');
    expect(main).toContain("`Searching ${sessionSearchProjectDoneCount}/${sortedProjectItems.length} projects`");
    expect(main).toContain('parts.push(`${sessionSearchErrorCount} error${sessionSearchErrorCount === 1 ? \'\' : \'s\'}`);');
    expect(main).toContain('sessionSearchStatusParts.join(\' · \')');
    expect(main).not.toContain('className="chat-hub-summary-count"');
    expect(main).not.toContain('Prompt · turn');
    expect(main).not.toContain('matched prompt text');
    expect(main).not.toContain(') : row.result.source === \'prompt\' ? (');
    expect(main).not.toContain('session-search-result-meta');
    expect(main).not.toContain("row.result.source !== 'title'");
    expect(main).toContain('title={title}');

    const hubButtonBlock = styles.match(/\.chat-hub-summary-button \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(hubButtonBlock).toContain('white-space: nowrap;');
    expect(styles).not.toContain('.chat-hub-summary-label {\n  display: none;');
    expect(styles).not.toContain('.chat-hub-summary-count {');
  });

  test('keeps the Hub popover inside the left sidebar', () => {
    const projectRoot = path.join(__dirname, '..');
    const styles = fs.readFileSync(path.join(projectRoot, 'web', 'src', 'styles.css'), 'utf8');

    const sidebarPopoverBlock = styles.match(/\.sidebar-title-row \.chat-hub-popover \{[\s\S]*?\n\}/)?.[0] ?? '';
    expect(sidebarPopoverBlock).toContain('left: 12px;');
    expect(sidebarPopoverBlock).toContain('right: 12px;');
    expect(sidebarPopoverBlock).toContain('max-width: none;');
  });
});
