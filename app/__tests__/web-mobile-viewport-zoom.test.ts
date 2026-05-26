import fs from 'fs';
import path from 'path';
import { installMobileViewportZoomGuard } from '../web/src/services/mobileViewportZoomGuard';

function projectRoot(): string {
  return path.join(__dirname, '..');
}

function readIndexHtml(): string {
  return fs.readFileSync(path.join(projectRoot(), 'web', 'public', 'index.html'), 'utf8');
}

function readMain(): string {
  return fs.readFileSync(path.join(projectRoot(), 'web', 'src', 'main.tsx'), 'utf8');
}

type ListenerEntry = {
  type: string;
  listener: EventListenerOrEventListenerObject;
  options?: AddEventListenerOptions;
};

class RecordingEventTarget {
  entries: ListenerEntry[] = [];

  addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: AddEventListenerOptions,
  ): void {
    this.entries.push({type, listener, options});
  }

  removeEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    this.entries = this.entries.filter(entry => entry.type !== type || entry.listener !== listener);
  }

  dispatch(type: string, event: Event): void {
    for (const entry of this.entries.filter(item => item.type === type)) {
      if (typeof entry.listener === 'function') {
        entry.listener(event);
      } else {
        entry.listener.handleEvent(event);
      }
    }
  }
}

function createTouchEvent(touchCount: number): Event {
  let prevented = false;
  return {
    touches: {length: touchCount},
    preventDefault: () => {
      prevented = true;
    },
    get defaultPrevented() {
      return prevented;
    },
  } as unknown as Event;
}

function createCancelableEvent(): Event {
  let prevented = false;
  return {
    preventDefault: () => {
      prevented = true;
    },
    get defaultPrevented() {
      return prevented;
    },
  } as unknown as Event;
}

describe('mobile viewport zoom lock', () => {
  test('declares a non-scalable mobile viewport', () => {
    const html = readIndexHtml();
    const viewportContent = html.match(/<meta name="viewport" content="([^"]+)"/)?.[1] ?? '';

    expect(viewportContent).toContain('width=device-width');
    expect(viewportContent).toContain('initial-scale=1.0');
    expect(viewportContent).toContain('maximum-scale=1.0');
    expect(viewportContent).toContain('user-scalable=no');
  });

  test('prevents pinch zoom gestures without blocking single-touch scrolling', () => {
    const target = new RecordingEventTarget();
    const uninstall = installMobileViewportZoomGuard(target);

    expect(target.entries.find(entry => entry.type === 'touchstart')?.options).toMatchObject({passive: false});
    expect(target.entries.find(entry => entry.type === 'touchmove')?.options).toMatchObject({passive: false});
    expect(target.entries.find(entry => entry.type === 'gesturestart')?.options).toMatchObject({passive: false});
    expect(target.entries.find(entry => entry.type === 'gesturechange')?.options).toMatchObject({passive: false});
    expect(target.entries.find(entry => entry.type === 'gestureend')?.options).toMatchObject({passive: false});

    const singleTouch = createTouchEvent(1);
    target.dispatch('touchstart', singleTouch);
    target.dispatch('touchmove', singleTouch);
    expect(singleTouch.defaultPrevented).toBe(false);

    const multiTouch = createTouchEvent(2);
    target.dispatch('touchstart', multiTouch);
    target.dispatch('touchmove', multiTouch);
    expect(multiTouch.defaultPrevented).toBe(true);

    const gestureStart = createCancelableEvent();
    target.dispatch('gesturestart', gestureStart);
    expect(gestureStart.defaultPrevented).toBe(true);

    uninstall();
    expect(target.entries).toHaveLength(0);
  });

  test('installs the zoom guard during web startup', () => {
    const main = readMain();

    expect(main.includes("import { installMobileViewportZoomGuard } from './services/mobileViewportZoomGuard';")).toBe(true);
    expect(main.includes('installMobileViewportZoomGuard(document);')).toBe(true);
  });
});
