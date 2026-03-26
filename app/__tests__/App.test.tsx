/**
 * @format
 */

import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import App from '../App';
import { Text, TextInput } from 'react-native';

const mockCreateObserveRepository = jest.fn();

jest.mock('../src/services/observeRepository', () => ({
  createObserveRepository: () => mockCreateObserveRepository(),
}));

type MockRepo = {
  initialize: jest.Mock<Promise<void>, [string, string?]>;
  listProjects: jest.Mock<Promise<Array<{ projectId: string; name: string }>>, []>;
  listFiles: jest.Mock<
    Promise<Array<{ name: string; path: string; kind: 'dir' | 'file' }>>,
    [string, string?]
  >;
  readFile: jest.Mock<Promise<string>, [string, string]>;
  close: jest.Mock<void, []>;
};

function createMockRepo(): MockRepo {
  return {
    initialize: jest.fn().mockResolvedValue(undefined),
    listProjects: jest.fn().mockResolvedValue([
      { projectId: 'p1', name: 'Project One' },
      { projectId: 'p2', name: 'Project Two' },
    ]),
    listFiles: jest.fn().mockResolvedValue([
      { name: 'src', path: '/src', kind: 'dir' },
      { name: 'README.md', path: '/README.md', kind: 'file' },
    ]),
    readFile: jest.fn().mockResolvedValue('hello'),
    close: jest.fn(),
  };
}

test('shows connect screen before session starts', async () => {
  mockCreateObserveRepository.mockReturnValue(createMockRepo());
  let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(() => {
    renderer = ReactTestRenderer.create(<App />);
  });

  expect(renderer).toBeDefined();
  const texts = renderer!.root.findAllByType(Text).map(node => String(node.props.children));
  expect(texts).toContain('Connect to WheelMaker');
});

test('connects and enters workspace with first project selected', async () => {
  const repo = createMockRepo();
  mockCreateObserveRepository.mockReturnValue(repo);

  let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(() => {
    renderer = ReactTestRenderer.create(<App />);
  });

  const inputs = renderer!.root.findAllByType(TextInput);
  await ReactTestRenderer.act(() => {
    inputs[0].props.onChangeText('127.0.0.1');
    inputs[1].props.onChangeText('secret');
  });

  const connectButton = renderer!.root.findAll(
    node => typeof node.props?.onPress === 'function',
  )[0];
  expect(connectButton).toBeDefined();

  await ReactTestRenderer.act(async () => {
    await connectButton!.props.onPress();
  });

  expect(repo.initialize).toHaveBeenCalledWith('ws://127.0.0.1:9630/ws', 'secret');
  expect(repo.listProjects).toHaveBeenCalled();
  expect(repo.listFiles).toHaveBeenCalledWith('p1', '.');

});
