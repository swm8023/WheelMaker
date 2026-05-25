import type {RegistryChatSession, RegistryProject} from '../types/registry';

export type MobileChatQuickSwitchSection = {
  projectId: string;
  projectName: string;
  sessions: RegistryChatSession[];
};

type BuildMobileChatQuickSwitchSectionsInput = {
  projects: RegistryProject[];
  sessionsByProjectId: Record<string, RegistryChatSession[]>;
  limit?: number;
};

function chatQuickSwitchSessionKey(projectId: string, sessionId: string): string {
  return `${projectId}\n${sessionId}`;
}

function isPriorityQuickSwitchSession(session: RegistryChatSession): boolean {
  return (session.unreadCount ?? 0) > 0 || session.running === true;
}

export function buildMobileChatQuickSwitchSections({
  projects,
  sessionsByProjectId,
  limit = 5,
}: BuildMobileChatQuickSwitchSectionsInput): MobileChatQuickSwitchSection[] {
  const priorityKeys: string[] = [];
  const fallbackKeys: string[] = [];
  const seen = new Set<string>();

  for (const project of projects) {
    const projectId = project.projectId;
    if (!projectId) {
      continue;
    }
    for (const session of sessionsByProjectId[projectId] ?? []) {
      if (!session.sessionId) {
        continue;
      }
      const key = chatQuickSwitchSessionKey(projectId, session.sessionId);
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      if (isPriorityQuickSwitchSession(session)) {
        priorityKeys.push(key);
      } else {
        fallbackKeys.push(key);
      }
    }
  }

  const selectedKeys = new Set([...priorityKeys, ...fallbackKeys].slice(0, Math.max(0, limit)));
  if (selectedKeys.size === 0) {
    return [];
  }

  const sections: MobileChatQuickSwitchSection[] = [];
  for (const project of projects) {
    const projectId = project.projectId;
    if (!projectId) {
      continue;
    }
    const sessions = (sessionsByProjectId[projectId] ?? [])
      .filter(session => session.sessionId && selectedKeys.has(chatQuickSwitchSessionKey(projectId, session.sessionId)));
    if (sessions.length === 0) {
      continue;
    }
    sections.push({
      projectId,
      projectName: project.name || projectId,
      sessions,
    });
  }
  return sections;
}
