import type {
  RegistryChatSession,
  RegistryProject,
  RegistrySessionSearchResult,
} from '../types/registry';

export const SESSION_SEARCH_FAST_POLL_MS = 300;
export const SESSION_SEARCH_SLOW_POLL_MS = 800;
const SESSION_SEARCH_SLOW_AFTER_UNCHANGED_POLLS = 3;

export type SessionSearchPollDelayInput = {
  changed: boolean;
  unchangedPolls: number;
};

export type SessionSearchResultsByProjectId = Record<string, RegistrySessionSearchResult[]>;

export type SessionSearchHighlightSegment = {
  text: string;
  match: boolean;
};

export type SessionSearchSectionRow = {
  session: RegistryChatSession;
  result: RegistrySessionSearchResult;
};

export type SessionSearchSection = {
  project: RegistryProject;
  rows: SessionSearchSectionRow[];
};

export function resolveSessionSearchPollDelay(input: SessionSearchPollDelayInput): number {
  if (input.changed || input.unchangedPolls < SESSION_SEARCH_SLOW_AFTER_UNCHANGED_POLLS) {
    return SESSION_SEARCH_FAST_POLL_MS;
  }
  return SESSION_SEARCH_SLOW_POLL_MS;
}

export function sameSessionSearchResults(
  left: RegistrySessionSearchResult[] = [],
  right: RegistrySessionSearchResult[] = [],
): boolean {
  if (left.length !== right.length) {
    return false;
  }
  for (let index = 0; index < left.length; index += 1) {
    const a = left[index];
    const b = right[index];
    if (
      a.projectId !== b.projectId ||
      a.sessionId !== b.sessionId ||
      a.source !== b.source ||
      (a.turnIndex ?? 0) !== (b.turnIndex ?? 0)
    ) {
      return false;
    }
  }
  return true;
}

export function mergeSessionSearchResultsByProject(
  current: SessionSearchResultsByProjectId,
  projectId: string,
  results: RegistrySessionSearchResult[],
): {resultsByProjectId: SessionSearchResultsByProjectId; changed: boolean} {
  const previous = current[projectId] ?? [];
  const nextResults = results.map(result => ({...result}));
  const changed = !sameSessionSearchResults(previous, nextResults);
  if (!changed) {
    return {resultsByProjectId: current, changed: false};
  }
  return {
    resultsByProjectId: {
      ...current,
      [projectId]: nextResults,
    },
    changed: true,
  };
}

export function buildSessionSearchSections(input: {
  projects: RegistryProject[];
  sessionsByProjectId: Record<string, RegistryChatSession[]>;
  resultsByProjectId: SessionSearchResultsByProjectId;
}): SessionSearchSection[] {
  return input.projects
    .map(project => {
      const resultBySessionId = new Map<string, RegistrySessionSearchResult>();
      for (const result of input.resultsByProjectId[project.projectId] ?? []) {
        if (!resultBySessionId.has(result.sessionId)) {
          resultBySessionId.set(result.sessionId, result);
        }
      }
      const rows = (input.sessionsByProjectId[project.projectId] ?? [])
        .map(session => {
          const result = resultBySessionId.get(session.sessionId);
          return result ? {session, result} : null;
        })
        .filter((item): item is SessionSearchSectionRow => item !== null);
      return rows.length > 0 ? {project, rows} : null;
    })
    .filter((item): item is SessionSearchSection => item !== null);
}

export function splitSessionSearchTitleHighlight(title: string, query: string): SessionSearchHighlightSegment[] {
  const normalizedQuery = query.trim().toLowerCase();
  if (!title || !normalizedQuery) {
    return title ? [{text: title, match: false}] : [];
  }
  const lowerTitle = title.toLowerCase();
  const segments: SessionSearchHighlightSegment[] = [];
  let cursor = 0;
  while (cursor < title.length) {
    const matchIndex = lowerTitle.indexOf(normalizedQuery, cursor);
    if (matchIndex < 0) {
      segments.push({text: title.slice(cursor), match: false});
      break;
    }
    if (matchIndex > cursor) {
      segments.push({text: title.slice(cursor, matchIndex), match: false});
    }
    segments.push({
      text: title.slice(matchIndex, matchIndex + normalizedQuery.length),
      match: true,
    });
    cursor = matchIndex + normalizedQuery.length;
  }
  return segments.filter(segment => segment.text.length > 0);
}
