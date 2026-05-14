export type ProjectNavigationItem = {
  projectId: string;
};

export function togglePinnedProjectId(current: string[], projectId: string): string[] {
  if (!projectId) {
    return current;
  }
  if (current.includes(projectId)) {
    return current.filter(item => item !== projectId);
  }
  return [...current, projectId];
}

export function sortProjectsByPin<T extends ProjectNavigationItem>(
  projects: T[],
  pinnedProjectIds: string[],
): T[] {
  const pinned = new Set(pinnedProjectIds);
  return projects
    .map((project, index) => ({project, index}))
    .sort((left, right) => {
      const leftPinned = pinned.has(left.project.projectId);
      const rightPinned = pinned.has(right.project.projectId);
      if (leftPinned !== rightPinned) {
        return leftPinned ? -1 : 1;
      }
      return left.index - right.index;
    })
    .map(entry => entry.project);
}
