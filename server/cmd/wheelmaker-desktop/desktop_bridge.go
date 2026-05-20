package main

const (
	desktopResourceIconID     uint = 1
	desktopTitleBarThemeColor      = "#1e1e1e"

	desktopStartDragBinding      = "__wheelMakerDesktopStartDrag"
	desktopMinimizeBinding       = "__wheelMakerDesktopMinimize"
	desktopToggleMaximizeBinding = "__wheelMakerDesktopToggleMaximize"
	desktopCloseBinding          = "__wheelMakerDesktopClose"
)

func desktopRuntimeInitScript() string {
	return `(() => {
  const invoke = name => () => {
    const fn = window[name];
    if (typeof fn === 'function') {
      return fn();
    }
    return Promise.resolve();
  };
  window.WheelMakerDesktop = Object.freeze({
    enabled: true,
    startDrag: invoke('` + desktopStartDragBinding + `'),
    minimize: invoke('` + desktopMinimizeBinding + `'),
    toggleMaximize: invoke('` + desktopToggleMaximizeBinding + `'),
    close: invoke('` + desktopCloseBinding + `'),
  });
})();`
}
