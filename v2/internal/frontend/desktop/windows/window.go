//go:build windows

package windows

import (
	"syscall"
	"unsafe"

	"github.com/leaanthony/winc"
	"github.com/leaanthony/winc/w32"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/options"
)

type Window struct {
	winc.Form
	frontendOptions                          *options.App
	applicationMenu                          *menu.Menu
	notifyParentWindowPositionChanged        func() error
	minWidth, minHeight, maxWidth, maxHeight int
}

func NewWindow(parent winc.Controller, appoptions *options.App) *Window {
	result := &Window{
		frontendOptions: appoptions,
		minHeight:       appoptions.MinHeight,
		minWidth:        appoptions.MinWidth,
		maxHeight:       appoptions.MaxHeight,
		maxWidth:        appoptions.MaxWidth,
	}
	result.SetIsForm(true)

	var exStyle int
	if appoptions.Windows != nil {
		exStyle = w32.WS_EX_CONTROLPARENT | w32.WS_EX_APPWINDOW
		if appoptions.Windows.WindowIsTranslucent {
			exStyle |= w32.WS_EX_NOREDIRECTIONBITMAP
		}
	}
	if appoptions.AlwaysOnTop {
		exStyle |= w32.WS_EX_TOPMOST
	}

	var dwStyle = w32.WS_THICKFRAME | w32.WS_SYSMENU | w32.WS_MAXIMIZEBOX | w32.WS_MINIMIZEBOX

	winc.RegClassOnlyOnce("wailsWindow")
	result.SetHandle(winc.CreateWindow("wailsWindow", parent, uint(exStyle), uint(dwStyle)))
	winc.RegMsgHandler(result)
	result.SetParent(parent)

	loadIcon := true
	if appoptions.Windows != nil && appoptions.Windows.DisableWindowIcon == true {
		loadIcon = false
	}
	if loadIcon {
		if ico, err := winc.NewIconFromResource(winc.GetAppInstance(), uint16(winc.AppIconID)); err == nil {
			result.SetIcon(0, ico)
		}
	}

	result.SetSize(appoptions.Width, appoptions.Height)
	result.SetText(appoptions.Title)
	result.EnableSizable(!appoptions.DisableResize)
	if !appoptions.Fullscreen {
		result.EnableMaxButton(!appoptions.DisableResize)
		result.SetMinSize(appoptions.MinWidth, appoptions.MinHeight)
		result.SetMaxSize(appoptions.MaxWidth, appoptions.MaxHeight)
	}

	if appoptions.Windows != nil {
		if appoptions.Windows.WindowIsTranslucent {
			result.SetTranslucentBackground()
		}

		if appoptions.Windows.DisableWindowIcon {
			result.DisableIcon()
		}
	}

	// Dlg forces display of focus rectangles, as soon as the user starts to type.
	w32.SendMessage(result.Handle(), w32.WM_CHANGEUISTATE, w32.UIS_INITIALIZE, 0)

	result.SetFont(winc.DefaultFont)

	if appoptions.Menu != nil {
		result.SetApplicationMenu(appoptions.Menu)
	}

	return result
}

func (w *Window) Run() int {
	return winc.RunMainLoop()
}

func (w *Window) Fullscreen() {
	w.Form.SetMaxSize(0, 0)
	w.Form.SetMinSize(0, 0)
	w.Form.Fullscreen()
}

func (w *Window) UnFullscreen() {
	if !w.IsFullScreen() {
		return
	}
	w.Form.UnFullscreen()
	w.SetMinSize(w.minWidth, w.minHeight)
	w.SetMaxSize(w.maxWidth, w.maxHeight)
}

func (w *Window) SetMinSize(minWidth int, minHeight int) {
	w.minWidth = minWidth
	w.minHeight = minHeight
	w.Form.SetMinSize(minWidth, minHeight)
}

func (w *Window) SetMaxSize(maxWidth int, maxHeight int) {
	w.maxWidth = maxWidth
	w.maxHeight = maxHeight
	w.Form.SetMaxSize(maxWidth, maxHeight)
}

type NCCALCSIZE_PARAMS struct {
	rgrc  [3]w32.RECT
	lppos uintptr /* WINDOWPOS */
}

func (w *Window) WndProc(msg uint32, wparam, lparam uintptr) uintptr {

	switch msg {
	case w32.WM_NCLBUTTONDOWN:
		w32.SetFocus(w.Handle())
	case w32.WM_MOVE, w32.WM_MOVING:
		if w.notifyParentWindowPositionChanged != nil {
			w.notifyParentWindowPositionChanged()
		}

	// TODO move WM_DPICHANGED handling into winc
	case 0x02E0: //w32.WM_DPICHANGED
		newWindowSize := (*w32.RECT)(unsafe.Pointer(lparam))
		w32.SetWindowPos(w.Handle(),
			uintptr(0),
			int(newWindowSize.Left),
			int(newWindowSize.Top),
			int(newWindowSize.Right-newWindowSize.Left),
			int(newWindowSize.Bottom-newWindowSize.Top),
			w32.SWP_NOZORDER|w32.SWP_NOACTIVATE)
	}

	if w.frontendOptions.Frameless {
		switch msg {
		case w32.WM_CREATE:

			sizeRect := w32.GetWindowRect(w.Handle())

			w32.SetWindowPos(w.Handle(), 0,
				int(sizeRect.Left),
				int(sizeRect.Top),
				int(sizeRect.Right-sizeRect.Left),
				int(sizeRect.Bottom-sizeRect.Top),
				w32.SWP_FRAMECHANGED|w32.SWP_NOMOVE|w32.SWP_NOSIZE)

			break
		case w32.WM_NCHITTEST:
			hit := w.Form.WndProc(msg, wparam, lparam)

			switch hit {
			case w32.HTNOWHERE:
			case w32.HTRIGHT:
			case w32.HTLEFT:
			case w32.HTTOPLEFT:
			case w32.HTTOP:
			case w32.HTTOPRIGHT:
			case w32.HTBOTTOMRIGHT:
			case w32.HTBOTTOM:
			case w32.HTBOTTOMLEFT:
				return hit
			}

			monitor := w32.MonitorFromWindow(w.Handle(), w32.MONITOR_DEFAULTTONEAREST)
			var monitorInfo w32.MONITORINFO
			monitorInfo.CbSize = uint32(unsafe.Sizeof(monitorInfo))
			if w32.GetMonitorInfo(monitor, &monitorInfo) {
				maxWidth := w.frontendOptions.MaxWidth
				maxHeight := w.frontendOptions.MaxHeight
				if maxWidth > 0 || maxHeight > 0 {
					var dpiX, dpiY uint
					w32.GetDPIForMonitor(monitor, w32.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)

					frameY := winc.ScaleWithDPI(w32.GetSystemMetrics(w32.SM_CYFRAME), dpiY)
					padding := winc.ScaleWithDPI(w32.GetSystemMetrics(92 /*SM_CXPADDEDBORDER */), dpiX)

					_, cursorY, ok := w32.ScreenToClient(w.Handle(), int(w32.LOWORD(uint32(lparam))), int(w32.HIWORD(uint32(lparam))))

					if ok {
						if cursorY > 0 && cursorY < frameY+padding {
							return w32.HTTOP
						}
					}

				}
			}

			return w32.HTCLIENT

		case w32.WM_NCCALCSIZE:
			// Disable the standard frame by allowing the client area to take the full
			// window size.
			// See: https://docs.microsoft.com/en-us/windows/win32/winmsg/wm-nccalcsize#remarks
			// This hides the titlebar and also disables the resizing from user interaction because the standard frame is not
			// shown. We still need the WS_THICKFRAME style to enable resizing from the frontend.
			if wparam != 0 {
				style := uint32(w32.GetWindowLong(w.Handle(), w32.GWL_STYLE))

				monitor := w32.MonitorFromWindow(w.Handle(), w32.MONITOR_DEFAULTTONEAREST)

				var monitorInfo w32.MONITORINFO
				monitorInfo.CbSize = uint32(unsafe.Sizeof(monitorInfo))
				if w32.GetMonitorInfo(monitor, &monitorInfo) {
					maxWidth := w.frontendOptions.MaxWidth
					maxHeight := w.frontendOptions.MaxHeight
					if maxWidth > 0 || maxHeight > 0 {
						var dpiX, dpiY uint
						w32.GetDPIForMonitor(monitor, w32.MDT_EFFECTIVE_DPI, &dpiX, &dpiY)

						frameX := winc.ScaleWithDPI(w32.GetSystemMetrics(w32.SM_CXFRAME), dpiX)
						frameY := winc.ScaleWithDPI(w32.GetSystemMetrics(w32.SM_CYFRAME), dpiY)

						// should we scale with dpiX or dpiY?
						padding := winc.ScaleWithDPI(w32.GetSystemMetrics(92 /*SM_CXPADDEDBORDER */), dpiX)

						params := (*NCCALCSIZE_PARAMS)(unsafe.Pointer(lparam))

						params.rgrc[0].Left += int32(frameX + padding)
						params.rgrc[0].Right -= int32(frameX + padding)
						params.rgrc[0].Bottom -= int32(frameY + padding)

						if style&w32.WS_MAXIMIZE != 0 {
							params.rgrc[0].Top += int32(padding)
						}

						return 0

					}
				}

				return 0
			}
		}
	}
	return w.Form.WndProc(msg, wparam, lparam)
}

// TODO this should be put into the winc if we are happy with this solution.
var (
	modkernel32                      = syscall.NewLazyDLL("dwmapi.dll")
	procDwmExtendFrameIntoClientArea = modkernel32.NewProc("DwmExtendFrameIntoClientArea")
)

func dwmExtendFrameIntoClientArea(hwnd w32.HWND, margins w32.MARGINS) error {
	ret, _, _ := procDwmExtendFrameIntoClientArea.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&margins)))

	if ret != 0 {
		return syscall.GetLastError()
	}

	return nil
}
