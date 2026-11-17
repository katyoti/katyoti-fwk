// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 calls x/sys/windows does not wrap.
var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procGetWindow          = user32.NewProc("GetWindow")
	procIsIconic           = user32.NewProc("IsIconic")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBM = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procGetDIBits          = gdi32.NewProc("GetDIBits")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	gwOwner = 4
	// pwClientOnly|pwRenderFullContent: the client area, composed content
	// included — without the latter a WebView2 window prints black.
	pwClientOnly        = 0x1
	pwRenderFullContent = 0x2
	biRGB               = 0
	dibRGBColors        = 0
)

type rect struct{ Left, Top, Right, Bottom int32 }

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// captureOwnWindowPNG prints this process's main window's client area — the
// web content — into a bitmap and encodes it as PNG. No picker, and the window
// need not be on top.
func captureOwnWindowPNG() ([]byte, error) {
	hwnd, err := ownMainWindow()
	if err != nil {
		return nil, err
	}
	var rc rect
	if r, _, e := procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc))); r == 0 {
		return nil, fmt.Errorf("GetClientRect: %w", e)
	}
	w, h := rc.Right-rc.Left, rc.Bottom-rc.Top
	if w <= 0 || h <= 0 {
		return nil, errors.New("the window has no visible area")
	}

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("GetDC failed")
	}
	defer procReleaseDC.Call(0, screenDC)
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, errors.New("CreateCompatibleDC failed")
	}
	defer procDeleteDC.Call(memDC)
	bm, _, _ := procCreateCompatibleBM.Call(screenDC, uintptr(w), uintptr(h))
	if bm == 0 {
		return nil, errors.New("CreateCompatibleBitmap failed")
	}
	defer procDeleteObject.Call(bm)
	old, _, _ := procSelectObject.Call(memDC, bm)
	defer procSelectObject.Call(memDC, old)

	if r, _, e := procPrintWindow.Call(uintptr(hwnd), memDC, pwClientOnly|pwRenderFullContent); r == 0 {
		return nil, fmt.Errorf("PrintWindow: %w", e)
	}

	// Read the pixels top-down (negative height), 32-bit BGRA.
	hdr := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32, Compression: biRGB}
	hdr.Size = uint32(unsafe.Sizeof(hdr))
	img := image.NewRGBA(image.Rect(0, 0, int(w), int(h)))
	if r, _, e := procGetDIBits.Call(memDC, bm, 0, uintptr(h),
		uintptr(unsafe.Pointer(&img.Pix[0])), uintptr(unsafe.Pointer(&hdr)), dibRGBColors); r == 0 {
		return nil, fmt.Errorf("GetDIBits: %w", e)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i] // BGRA → RGBA
		img.Pix[i+3] = 0xff
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode screenshot: %w", err)
	}
	return buf.Bytes(), nil
}

// ownMainWindow is this process's main window: the foreground window when it
// is ours, else the largest visible, unowned, non-minimized top-level window
// of this process.
func ownMainWindow() (windows.HWND, error) {
	if fg := windows.GetForegroundWindow(); fg != 0 && isOwnMainWindow(fg) {
		return fg, nil
	}
	enumMu.Lock()
	defer enumMu.Unlock()
	enumBest, enumBestArea = 0, 0
	_ = windows.EnumWindows(enumCallback(), nil)
	if enumBest == 0 {
		return 0, errors.New("no visible window to capture")
	}
	return enumBest, nil
}

func isOwnMainWindow(h windows.HWND) bool {
	var p uint32
	if _, err := windows.GetWindowThreadProcessId(h, &p); err != nil || p != uint32(os.Getpid()) {
		return false
	}
	if !windows.IsWindowVisible(h) {
		return false
	}
	if owner, _, _ := procGetWindow.Call(uintptr(h), gwOwner); owner != 0 {
		return false
	}
	iconic, _, _ := procIsIconic.Call(uintptr(h))
	return iconic == 0
}

// The EnumWindows callback is created once: syscall.NewCallback slots are
// never freed. Its result lives in the enum* variables, under enumMu.
var (
	enumMu       sync.Mutex
	enumBest     windows.HWND
	enumBestArea int64
	enumCallback = sync.OnceValue(func() uintptr {
		return syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
			if isOwnMainWindow(h) {
				var rc rect
				if r, _, _ := procGetClientRect.Call(uintptr(h), uintptr(unsafe.Pointer(&rc))); r != 0 {
					if a := int64(rc.Right-rc.Left) * int64(rc.Bottom-rc.Top); a > enumBestArea {
						enumBest, enumBestArea = h, a
					}
				}
			}
			return 1 // continue
		})
	})
)
