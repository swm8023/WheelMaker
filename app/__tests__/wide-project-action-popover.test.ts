import { resolveWideProjectActionPopoverPlacement } from '../web/src/chat/wideProjectActionPopover';

describe('wide project action popover placement', () => {
  test('flips above the project action when there is not enough space below', () => {
    const placement = resolveWideProjectActionPopoverPlacement({
      anchorRect: {
        top: 520,
        right: 454,
        bottom: 548,
      },
      viewportWidth: 1024,
      viewportHeight: 600,
      preferredWidth: 260,
      preferredMaxHeight: 240,
    });

    expect(placement).toEqual({
      placement: 'above',
      top: 516,
      left: 194,
      width: 260,
      maxHeight: 240,
    });
  });

  test('keeps the popover inside the viewport horizontally', () => {
    const placement = resolveWideProjectActionPopoverPlacement({
      anchorRect: {
        top: 80,
        right: 220,
        bottom: 108,
      },
      viewportWidth: 240,
      viewportHeight: 640,
      preferredWidth: 260,
      preferredMaxHeight: 240,
    });

    expect(placement).toEqual({
      placement: 'below',
      top: 112,
      left: 8,
      width: 224,
      maxHeight: 240,
    });
  });

  test('can place a context menu from the pointer instead of offsetting by menu width', () => {
    const placement = resolveWideProjectActionPopoverPlacement({
      anchorRect: {
        top: 200,
        right: 320,
        bottom: 200,
      },
      viewportWidth: 800,
      viewportHeight: 600,
      preferredWidth: 156,
      preferredMaxHeight: 190,
      align: 'start',
    });

    expect(placement).toEqual({
      placement: 'below',
      top: 204,
      left: 320,
      width: 156,
      maxHeight: 190,
    });
  });
});
