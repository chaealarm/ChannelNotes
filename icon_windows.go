package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

const customIconFile = "program-icon.png"

func (a *App) iconPath() string { return filepath.Join(a.dir, customIconFile) }

func (a *App) iconBytes() []byte {
	if a.dir != "" {
		if data, err := os.ReadFile(a.iconPath()); err == nil && len(data) != 0 {
			if _, err := png.DecodeConfig(bytes.NewReader(data)); err == nil {
				return data
			}
		}
	}
	return defaultIcon
}

func iconDataURL(data []byte) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
}

func (a *App) GetProgramIcon() string { return iconDataURL(a.iconBytes()) }

func (a *App) ChooseProgramIcon() (string, error) {
	path, err := a.openFile("프로그램 아이콘 선택", "PNG 이미지", "*.png")
	if err != nil || path == "" {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() == 0 || info.Size() > 5<<20 {
		return "", errors.New("아이콘 이미지는 5 MB 이하의 PNG 파일이어야 합니다")
	}
	data := make([]byte, info.Size())
	if _, err = io.ReadFull(file, data); err != nil {
		return "", err
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 16 || config.Height < 16 || config.Width > 2048 || config.Height > 2048 {
		return "", errors.New("16~2048픽셀 PNG 아이콘을 선택하세요")
	}
	if err = a.setProgramIcon(data); err != nil {
		return "", err
	}
	return iconDataURL(data), nil
}

func (a *App) ResetProgramIcon() (string, error) {
	if err := a.setProgramIcon(defaultIcon); err != nil {
		return "", err
	}
	if err := os.Remove(a.iconPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return iconDataURL(defaultIcon), nil
}

func (a *App) setProgramIcon(data []byte) error {
	small, err := w32.CreateSmallHIconFromImage(data)
	if err != nil {
		return fmt.Errorf("아이콘 적용 실패: %w", err)
	}
	large, err := w32.CreateLargeHIconFromImage(data)
	if err != nil {
		w32.DestroyIcon(small)
		return fmt.Errorf("아이콘 적용 실패: %w", err)
	}
	a.mu.Lock()
	if !bytes.Equal(data, defaultIcon) {
		if err = os.WriteFile(a.iconPath(), data, 0644); err != nil {
			a.mu.Unlock()
			w32.DestroyIcon(small)
			w32.DestroyIcon(large)
			return err
		}
	}
	previousSmall, previousLarge := a.iconSmall, a.iconLarge
	a.iconSmall, a.iconLarge = small, large
	a.applyWindowIconUnlocked(a.mainWindow)
	for _, entry := range a.detached {
		a.applyWindowIconUnlocked(entry.window)
	}
	a.mu.Unlock()
	if previousSmall != 0 {
		w32.DestroyIcon(previousSmall)
	}
	if previousLarge != 0 {
		w32.DestroyIcon(previousLarge)
	}
	a.wails.Event.Emit("program:icon", iconDataURL(data))
	return nil
}

func (a *App) applyWindowIcons() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.iconSmall == 0 || a.iconLarge == 0 {
		data := a.iconBytes()
		small, smallErr := w32.CreateSmallHIconFromImage(data)
		large, largeErr := w32.CreateLargeHIconFromImage(data)
		if smallErr != nil || largeErr != nil {
			if small != 0 {
				w32.DestroyIcon(small)
			}
			if large != 0 {
				w32.DestroyIcon(large)
			}
			return
		}
		a.iconSmall, a.iconLarge = small, large
	}
	a.applyWindowIconUnlocked(a.mainWindow)
}

func (a *App) applyWindowIconUnlocked(window *application.WebviewWindow) {
	if window == nil || window.NativeWindow() == nil || a.iconSmall == 0 || a.iconLarge == 0 {
		return
	}
	hwnd := w32.HWND(uintptr(unsafe.Pointer(window.NativeWindow())))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, uintptr(a.iconSmall))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, uintptr(a.iconLarge))
}

func (a *App) releaseIconsUnlocked() {
	if a.iconSmall != 0 {
		w32.DestroyIcon(a.iconSmall)
		a.iconSmall = 0
	}
	if a.iconLarge != 0 {
		w32.DestroyIcon(a.iconLarge)
		a.iconLarge = 0
	}
}
