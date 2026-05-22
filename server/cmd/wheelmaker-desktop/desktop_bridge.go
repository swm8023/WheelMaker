package main

const (
	desktopResourceIconID     uint = 1
	desktopTitleBarThemeColor      = "#1e1e1e"

	desktopStartDragBinding      = "__wheelMakerDesktopStartDrag"
	desktopMinimizeBinding       = "__wheelMakerDesktopMinimize"
	desktopToggleMaximizeBinding = "__wheelMakerDesktopToggleMaximize"
	desktopCloseBinding          = "__wheelMakerDesktopClose"
	desktopGetWebSourceBinding   = "__wheelMakerDesktopGetWebSourceState"
	desktopSetWebSourceBinding   = "__wheelMakerDesktopSetWebSourcePreference"
	desktopSetRemoteWebBinding   = "__wheelMakerDesktopSetRemoteWebCandidate"
)

func desktopRuntimeInitScript() string {
	return `(() => {
  const invoke = name => (...args) => {
    const fn = window[name];
    if (typeof fn === 'function') {
      return fn(...args);
    }
    return Promise.resolve();
  };
  window.WheelMakerDesktop = Object.freeze({
    enabled: true,
    startDrag: invoke('` + desktopStartDragBinding + `'),
    minimize: invoke('` + desktopMinimizeBinding + `'),
    toggleMaximize: invoke('` + desktopToggleMaximizeBinding + `'),
    close: invoke('` + desktopCloseBinding + `'),
    getWebSourceState: invoke('` + desktopGetWebSourceBinding + `'),
    setWebSourcePreference: invoke('` + desktopSetWebSourceBinding + `'),
    setRemoteWebCandidate: invoke('` + desktopSetRemoteWebBinding + `'),
  });
})();`
}
