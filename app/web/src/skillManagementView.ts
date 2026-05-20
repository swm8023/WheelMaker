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

export function deriveSkillHubIds(hubs: RegistryHub[]): string[] {
  return Array.from(new Set(
    hubs
      .map(hub => (hub.hubId || '').trim())
      .filter(Boolean),
  )).sort((left, right) => left.localeCompare(right));
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

export function skillScopeLabel(input: {scope: RegistrySkillScope; hubId: string; projectName?: string}): string {
  if (input.scope === 'project') return `Project: ${input.projectName || ''}`.trim();
  return `Hub: ${input.hubId}`;
}
