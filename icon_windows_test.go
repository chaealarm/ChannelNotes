package main

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProgramIconPrefersSavedUserIcon(t *testing.T) {
	a := NewApp()
	a.dir = t.TempDir()
	if !bytes.Equal(a.iconBytes(), defaultIcon) {
		t.Fatal("default icon was not used")
	}
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	custom := encoded.Bytes()
	if err := os.WriteFile(filepath.Join(a.dir, customIconFile), custom, 0644); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.iconBytes(), custom) {
		t.Fatal("saved user icon was not loaded")
	}
	url := a.GetProgramIcon()
	if !strings.HasPrefix(url, "data:image/png;base64,") || !strings.HasSuffix(url, base64.StdEncoding.EncodeToString(custom)) {
		t.Fatalf("unexpected icon data URL: %q", url)
	}
}
