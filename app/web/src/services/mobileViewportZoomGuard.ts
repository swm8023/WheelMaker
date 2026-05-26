export type MobileViewportZoomGuardTarget = {
  addEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: AddEventListenerOptions,
  ): void;
  removeEventListener(
    type: string,
    listener: EventListenerOrEventListenerObject,
    options?: EventListenerOptions,
  ): void;
};

type MultiTouchEvent = Event & {
  touches?: {
    length: number;
  };
};

const NON_PASSIVE_OPTIONS: AddEventListenerOptions = {passive: false};

function hasMultipleTouches(event: Event): boolean {
  const touches = (event as MultiTouchEvent).touches;
  return typeof touches?.length === 'number' && touches.length > 1;
}

export function installMobileViewportZoomGuard(target: MobileViewportZoomGuardTarget): () => void {
  const preventMultiTouchZoom = (event: Event) => {
    if (hasMultipleTouches(event)) {
      event.preventDefault();
    }
  };
  const preventGestureZoom = (event: Event) => {
    event.preventDefault();
  };

  target.addEventListener('touchstart', preventMultiTouchZoom, NON_PASSIVE_OPTIONS);
  target.addEventListener('touchmove', preventMultiTouchZoom, NON_PASSIVE_OPTIONS);
  target.addEventListener('gesturestart', preventGestureZoom, NON_PASSIVE_OPTIONS);
  target.addEventListener('gesturechange', preventGestureZoom, NON_PASSIVE_OPTIONS);
  target.addEventListener('gestureend', preventGestureZoom, NON_PASSIVE_OPTIONS);

  return () => {
    target.removeEventListener('touchstart', preventMultiTouchZoom, NON_PASSIVE_OPTIONS);
    target.removeEventListener('touchmove', preventMultiTouchZoom, NON_PASSIVE_OPTIONS);
    target.removeEventListener('gesturestart', preventGestureZoom, NON_PASSIVE_OPTIONS);
    target.removeEventListener('gesturechange', preventGestureZoom, NON_PASSIVE_OPTIONS);
    target.removeEventListener('gestureend', preventGestureZoom, NON_PASSIVE_OPTIONS);
  };
}
