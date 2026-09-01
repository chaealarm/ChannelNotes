package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderStoreRoundTripAndLazyContent(t *testing.T) {
	dir := t.TempDir()
	s := defaultStore()
	n := &s.Channels[0].Categories[0].Notes[0]
	n.ContentLoaded = true
	n.Content = "<h1>folder storage</h1>"
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadFolderStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Channels[0].Categories[0].Notes[0]
	if got.Content != "" || got.ContentLoaded {
		t.Fatal("metadata load eagerly loaded note content")
	}
	content, err := loadNoteFolder(dir, loaded, got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if content != n.Content {
		t.Fatalf("content mismatch: %q", content)
	}
	expected := filepath.Join(dataRoot(dir), "groups", s.Groups[0].ID, "channels", s.Channels[0].ID, "categories", s.Channels[0].Categories[0].ID, "notes", n.ID, "content.html")
	if _, err = os.Stat(expected); err != nil {
		t.Fatalf("folder hierarchy missing: %v", err)
	}
}

func TestFolderBackupRestore(t *testing.T) {
	source := t.TempDir()
	s := defaultStore()
	s.Channels[0].Categories[0].Notes[0].ContentLoaded = true
	if err := writeFolderStore(source, s, ""); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.zip")
	if err := zipFolder(dataRoot(source), backup); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	restored, err := restoreFolderZip(target, backup)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Groups) != 1 || len(restored.Channels) != 1 {
		t.Fatalf("unexpected restored metadata: %+v", restored)
	}
	n := restored.Channels[0].Categories[0].Notes[0]
	content, err := loadNoteFolder(target, restored, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if content == "" {
		t.Fatal("restored note content is empty")
	}
}

func TestFolderStoreRecoversWithoutSettings(t *testing.T) {
	dir := t.TempDir()
	s := defaultStore()
	originalGroupID := s.Groups[0].ID
	originalNoteID := s.Channels[0].Categories[0].Notes[0].ID
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dataRoot(dir), "settings.json")); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadFolderStore(dir)
	if err != nil {
		t.Fatalf("group hierarchy should recover without settings: %v", err)
	}
	if len(loaded.Groups) != 1 || loaded.Groups[0].ID != originalGroupID {
		t.Fatalf("existing group was not recovered: %+v", loaded.Groups)
	}
	if loaded.LastGroupID != originalGroupID || loaded.LastNoteID != originalNoteID {
		t.Fatalf("last selection was not repaired: %+v", loaded)
	}
	if loaded.Theme != "dark" || !loaded.ShowGroupPopup || !loaded.PeriodicAutoSave {
		t.Fatalf("safe settings defaults were not applied: %+v", loaded)
	}
}

func TestFolderStoreRecoversWithCorruptSettings(t *testing.T) {
	dir := t.TempDir()
	s := defaultStore()
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot(dir), "settings.json"), []byte("{broken"), 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadFolderStore(dir)
	if err != nil {
		t.Fatalf("group hierarchy should recover with corrupt settings: %v", err)
	}
	if len(loaded.Groups) != 1 || loaded.Groups[0].ID != s.Groups[0].ID {
		t.Fatalf("existing group was replaced: %+v", loaded.Groups)
	}
}
