package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFolderStoreRoundTripAndLazyContent(t *testing.T) {
	dir := t.TempDir()
	s := defaultStore()
	s.ImageInsertWidth = 37
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
	if loaded.ImageInsertWidth != 37 {
		t.Fatalf("image insert width was not persisted: %d", loaded.ImageInsertWidth)
	}
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

func TestPersistRestoredGroupWritesAllNoteBodies(t *testing.T) {
	dir := t.TempDir()
	original := defaultStore()
	if err := writeFolderStore(dir, original, ""); err != nil {
		t.Fatal(err)
	}

	restoredGroup := Group{ID: "restored-group", Name: "복원 그룹"}
	restoredChannel := Channel{ID: "restored-channel", Name: "복원 채널", GroupID: restoredGroup.ID, Categories: []Category{
		{ID: "restored-category-a", Name: "카테고리 A", Notes: []Note{
			{ID: "restored-note-a", Title: "메모 A", Name: "메모 A", Content: "<p>첫 번째 본문</p>", ContentLoaded: true},
			{ID: "restored-note-b", Title: "메모 B", Name: "메모 B", Content: "<p>두 번째 본문</p>", ContentLoaded: true},
		}},
		{ID: "restored-category-b", Name: "빈 카테고리", Notes: []Note{}},
	}}
	a := NewApp()
	a.dir = dir
	if err := a.persistRestoredGroup(GroupBundle{Group: restoredGroup, Channels: []Channel{restoredChannel}}); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadFolderStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Groups) != 2 || len(loaded.Channels) != 2 {
		t.Fatalf("restored hierarchy missing: groups=%d channels=%d", len(loaded.Groups), len(loaded.Channels))
	}
	var got *Channel
	for i := range loaded.Channels {
		if loaded.Channels[i].ID == restoredChannel.ID {
			got = &loaded.Channels[i]
		}
	}
	if got == nil || len(got.Categories) != 2 || len(got.Categories[0].Notes) != 2 {
		t.Fatalf("restored categories or notes missing: %+v", got)
	}
	for i, want := range []string{"<p>첫 번째 본문</p>", "<p>두 번째 본문</p>"} {
		content, loadErr := loadNoteFolder(dir, loaded, got.Categories[0].Notes[i].ID)
		if loadErr != nil || content != want {
			t.Fatalf("note %d body mismatch: content=%q err=%v", i, content, loadErr)
		}
	}
}

func TestPrepareRestoredGroupAssignsUniqueIDs(t *testing.T) {
	bundle := GroupBundle{Group: Group{ID: "old-group", Name: "백업"}}
	channel := Channel{ID: "old-channel", GroupID: "old-group", Name: "채널"}
	for categoryIndex := 0; categoryIndex < 30; categoryIndex++ {
		category := Category{ID: "same-old-category", Name: "카테고리"}
		for noteIndex := 0; noteIndex < 30; noteIndex++ {
			category.Notes = append(category.Notes, Note{ID: "same-old-note", Name: "메모", Content: "<p>보존할 본문</p>"})
		}
		channel.Categories = append(channel.Categories, category)
	}
	bundle.Channels = []Channel{channel}

	restored := prepareRestoredGroup(bundle)
	seen := map[string]bool{restored.Group.ID: true}
	for _, c := range restored.Channels {
		if c.GroupID != restored.Group.ID || seen[c.ID] {
			t.Fatalf("invalid or duplicate channel id: %q", c.ID)
		}
		seen[c.ID] = true
		for _, category := range c.Categories {
			if seen[category.ID] {
				t.Fatalf("duplicate category id: %q", category.ID)
			}
			seen[category.ID] = true
			for _, note := range category.Notes {
				if seen[note.ID] {
					t.Fatalf("duplicate note id: %q", note.ID)
				}
				seen[note.ID] = true
				if note.Content != "<p>보존할 본문</p>" || !note.ContentLoaded {
					t.Fatalf("note body was not preserved: %+v", note)
				}
			}
		}
	}
	if len(seen) != 1+1+30+900 {
		t.Fatalf("unexpected unique id count: %d", len(seen))
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

func TestFolderStorePreservesListOrder(t *testing.T) {
	dir := t.TempDir()
	s := defaultStore()
	c := s.Channels[0]
	c2 := c
	c2.ID = "channel-second"
	c2.Name = "second"
	c2.Categories = nil
	s.Channels = append([]Channel{c2}, c)
	s.Channels[1].Categories = []Category{
		{ID: "category-second", Name: "second", Notes: []Note{{ID: "note-b", Name: "B"}, {ID: "note-a", Name: "A"}}},
		{ID: "category-first", Name: "first", Notes: []Note{{ID: "note-c", Name: "C"}}},
	}
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadFolderStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Channels[0].ID != "channel-second" || loaded.Channels[1].ID != c.ID {
		t.Fatalf("channel order changed: %+v", loaded.Channels)
	}
	categories := loaded.Channels[1].Categories
	if categories[0].ID != "category-second" || categories[1].ID != "category-first" {
		t.Fatalf("category order changed: %+v", categories)
	}
	if categories[0].Notes[0].ID != "note-b" || categories[0].Notes[1].ID != "note-a" {
		t.Fatalf("note order changed: %+v", categories[0].Notes)
	}
}
