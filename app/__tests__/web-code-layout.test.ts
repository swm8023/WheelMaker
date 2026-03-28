import {getLineNumberDigits} from '../web/src/codeLayout';

describe('web code layout', () => {
  test('uses 1 digit gutter for short files', () => {
    expect(getLineNumberDigits(1)).toBe(1);
    expect(getLineNumberDigits(9)).toBe(1);
  });
});
