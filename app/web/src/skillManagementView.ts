import type {
  RegistryHub,
  RegistrySkillProjectSnapshot,
  RegistrySkillScope,
  RegistrySkillSnapshot,
} from './types/registry';

export interface SkillCategoryGroup {
  category: string;
  categoryKey: string;
  skills: RegistrySkillSnapshot[];
}

export interface ParsedSkillSourceInput {
  source: string;
  skillNames: string[];
}

export function deriveSkillHubIds(hubs: RegistryHub[]): string[] {
  return Array.from(new Set(
    hubs
      .map(hub => (hub.hubId || '').trim())
      .filter(Boolean),
  )).sort((left, right) => left.localeCompare(right));
}

export function parseSkillSourceInput(input: string): ParsedSkillSourceInput {
  const tokens = splitSkillSourceInput(input);
  if (tokens.length === 0) {
    return {source: '', skillNames: []};
  }

  const sourceIndex = findSkillSourceTokenIndex(tokens);
  const source = sourceIndex >= 0 ? tokens[sourceIndex] : '';
  return {
    source,
    skillNames: extractSkillNamesFromTokens(tokens),
  };
}

function splitSkillSourceInput(input: string): string[] {
  const tokens: string[] = [];
  let current = '';
  let quote: '"' | "'" | '' = '';
  let escaping = false;

  const pushCurrent = () => {
    if (current) {
      tokens.push(current);
      current = '';
    }
  };

  for (const char of input.trim()) {
    if (escaping) {
      current += char;
      escaping = false;
      continue;
    }
    if (char === '\\' && quote !== "'") {
      escaping = true;
      continue;
    }
    if (quote) {
      if (char === quote) {
        quote = '';
      } else {
        current += char;
      }
      continue;
    }
    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }
    if (/\s/.test(char)) {
      pushCurrent();
      continue;
    }
    current += char;
  }
  pushCurrent();
  return tokens;
}

function findSkillSourceTokenIndex(tokens: string[]): number {
  for (let index = 0; index < tokens.length - 2; index += 1) {
    if (tokens[index].toLowerCase() === 'skills' && tokens[index + 1].toLowerCase() === 'add') {
      return index + 2;
    }
  }
  return tokens.findIndex(token => token && !token.startsWith('-'));
}

function extractSkillNamesFromTokens(tokens: string[]): string[] {
  const names: string[] = [];
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index];
    if (token === '--skill') {
      for (let valueIndex = index + 1; valueIndex < tokens.length; valueIndex += 1) {
        const value = tokens[valueIndex];
        if (!value || value.startsWith('-')) {
          break;
        }
        names.push(value);
        index = valueIndex;
      }
      continue;
    }
    if (token.startsWith('--skill=')) {
      names.push(token.slice('--skill='.length));
    }
  }
  return Array.from(new Set(names.map(name => name.trim()).filter(Boolean)));
}

export function groupSkillsByCategory(skills: RegistrySkillSnapshot[]): SkillCategoryGroup[] {
  const groups = new Map<string, SkillCategoryGroup>();
  skills.forEach(skill => {
    const categoryKey = (skill.categoryKey || '').trim() || 'general';
    const category = (skill.category || '').trim() || 'General';
    const existing = groups.get(categoryKey) ?? {category, categoryKey, skills: []};
    existing.skills.push(skill);
    groups.set(categoryKey, existing);
  });
  return Array.from(groups.values())
    .map(group => ({
      ...group,
      skills: [...group.skills].sort((left, right) => left.name.localeCompare(right.name)),
    }))
    .sort((left, right) => {
      if (left.categoryKey === 'general') return 1;
      if (right.categoryKey === 'general') return -1;
      return left.category.localeCompare(right.category);
    });
}

export function sortSkillProjects(projects: RegistrySkillProjectSnapshot[]): RegistrySkillProjectSnapshot[] {
  return [...projects].sort((left, right) => {
    if (left.online !== right.online) return left.online ? -1 : 1;
    return left.projectName.localeCompare(right.projectName);
  });
}

export function skillOperationStatusLabel(status: string): string {
  switch (status) {
    case 'running':
      return 'Running';
    case 'succeeded':
      return 'Succeeded';
    case 'failed':
      return 'Failed';
    default:
      return status || 'Unknown';
  }
}

export function skillScopeLabel(input: {scope: RegistrySkillScope; hubId: string; projectName?: string}): string {
  if (input.scope === 'project') return `Project: ${input.projectName || ''}`.trim();
  return `Hub: ${input.hubId}`;
}

export function isSkillActionPendingForHub(pendingKey: string, hubId: string): boolean {
  const normalizedHubId = (hubId || '').trim();
  if (!pendingKey || !normalizedHubId) return false;
  return pendingKey.split(':', 1)[0] === normalizedHubId;
}
