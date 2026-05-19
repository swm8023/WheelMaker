import React from 'react';
import {
  DEFAULT_CHAT_TURN_HEIGHT_METRICS,
  type ChatTurnHeightMetrics,
} from './chatDisplayIndex';

const WIDTH_BUCKET_PX = 32;

function bucketWidth(width: number): number {
  return Math.max(240, Math.round(width / WIDTH_BUCKET_PX) * WIDTH_BUCKET_PX);
}

function numericStyle(style: CSSStyleDeclaration, property: string): number {
  const value = Number.parseFloat(style.getPropertyValue(property));
  return Number.isFinite(value) ? value : 0;
}

function measureContentWidth(element: HTMLElement): number {
  const style = window.getComputedStyle(element);
  const horizontalPadding =
    numericStyle(style, 'padding-left') + numericStyle(style, 'padding-right');
  return bucketWidth(element.clientWidth - horizontalPadding);
}

function metricsEqual(left: ChatTurnHeightMetrics, right: ChatTurnHeightMetrics): boolean {
  return Object.keys(DEFAULT_CHAT_TURN_HEIGHT_METRICS).every(key => {
    const metricKey = key as keyof ChatTurnHeightMetrics;
    return left[metricKey] === right[metricKey];
  });
}

function readChatLayoutMetrics(element: HTMLElement | null): ChatTurnHeightMetrics {
  if (!element || typeof window === 'undefined') {
    return DEFAULT_CHAT_TURN_HEIGHT_METRICS;
  }
  return {
    ...DEFAULT_CHAT_TURN_HEIGHT_METRICS,
    contentWidth: measureContentWidth(element),
  };
}

export function useChatLayoutMetrics(
  scrollRef: React.RefObject<HTMLElement | null>,
): ChatTurnHeightMetrics {
  const [metrics, setMetrics] = React.useState<ChatTurnHeightMetrics>(
    DEFAULT_CHAT_TURN_HEIGHT_METRICS,
  );

  React.useLayoutEffect(() => {
    const element = scrollRef.current;
    const update = () => {
      const next = readChatLayoutMetrics(element);
      setMetrics(current => (metricsEqual(current, next) ? current : next));
    };
    update();
    if (!element || typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', update);
      return () => window.removeEventListener('resize', update);
    }
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [scrollRef]);

  return metrics;
}
