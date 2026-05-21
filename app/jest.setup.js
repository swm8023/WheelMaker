globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const originalConsoleError = console.error;
console.error = (...args) => {
  const firstArg = args[0];
  if (
    typeof firstArg === 'string' &&
    firstArg.includes('react-test-renderer is deprecated')
  ) {
    return;
  }
  originalConsoleError(...args);
};

if (typeof globalThis.window === 'undefined') {
  globalThis.window = globalThis;
}

if (typeof globalThis.window.requestAnimationFrame !== 'function') {
  globalThis.window.requestAnimationFrame = callback =>
    setTimeout(() => callback(Date.now()), 0);
}

if (typeof globalThis.window.cancelAnimationFrame !== 'function') {
  globalThis.window.cancelAnimationFrame = id => clearTimeout(id);
}

if (typeof globalThis.window.navigator === 'undefined') {
  globalThis.window.navigator = globalThis.navigator ?? {userAgent: 'node'};
}
