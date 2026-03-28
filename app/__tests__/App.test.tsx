import React from 'react';
import ReactTestRenderer from 'react-test-renderer';
import {Text} from 'react-native';
import App from '../App.tsx';

test('renders fallback app placeholder', async () => {
  let renderer: ReactTestRenderer.ReactTestRenderer | undefined;
  await ReactTestRenderer.act(() => {
    renderer = ReactTestRenderer.create(<App />);
  });

  const texts = renderer!.root.findAllByType(Text).map(node => String(node.props.children));
  expect(texts).toContain('WheelMaker uses App.native.tsx for native shell.');
});
