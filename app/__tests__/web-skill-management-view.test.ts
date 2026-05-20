import {
  deriveSkillHubIds,
  groupSkillsByCategory,
  skillAgentsLabel,
  skillScopeLabel,
  sortSkillProjects,
} from '../web/src/skillManagementView';

describe('skill management view helpers', () => {
  test('derives sorted hub ids from project.list hubs', () => {
    expect(deriveSkillHubIds([{hubId: 'hub-b'}, {hubId: ' '}, {hubId: 'hub-a'}])).toEqual(['hub-a', 'hub-b']);
  });

  test('groups skills by upstream category and keeps General last', () => {
    const groups = groupSkillsByCategory([
      {name: 'plain', category: '', categoryKey: '', agents: []},
      {name: 'tdd', category: 'Mattpocock Skills', categoryKey: 'mattpocock-skills', agents: []},
    ]);

    expect(groups.map(group => group.category)).toEqual(['Mattpocock Skills', 'General']);
    expect(groups[0].skills[0].name).toBe('tdd');
  });

  test('formats omitted skill agents as no linked agents', () => {
    expect(skillAgentsLabel({
      name: 'plain',
      category: 'General',
      categoryKey: 'general',
    })).toBe('No linked agents');
  });

  test('sorts projects by online state then name', () => {
    expect(sortSkillProjects([
      {projectName: 'zeta', online: false, skills: []},
      {projectName: 'alpha', online: true, skills: []},
    ]).map(project => project.projectName)).toEqual(['alpha', 'zeta']);
  });

  test('formats scope labels', () => {
    expect(skillScopeLabel({scope: 'hub', hubId: 'hub-a'})).toBe('Hub: hub-a');
    expect(skillScopeLabel({scope: 'project', hubId: 'hub-a', projectName: 'WheelMaker'})).toBe('Project: WheelMaker');
  });
});
