package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func channelMoveStore() Store {
	s := defaultStore()
	s.Groups = append(s.Groups, Group{ID: "target-group", Name: "대상 그룹"})
	s.Channels = append(s.Channels, Channel{
		ID:      "moving-channel",
		Name:    "이동 채널",
		GroupID: s.Groups[0].ID,
		Categories: []Category{{
			ID: "moving-category", Name: "이동 카테고리",
			Notes: []Note{{ID: "moving-note", Title: "이동 메모", Name: "이동 메모", Content: "<p>내용</p>", ContentLoaded: true}},
		}},
	})
	return s
}

func TestMoveChannelToGroupMovesFolder(t *testing.T) {
	dir := t.TempDir()
	s := channelMoveStore()
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	a := &App{dir: dir, store: s}
	if err := os.MkdirAll(filepath.Join(dir, "locks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.AcquireGroup(s.Groups[0].ID); err != nil {
		t.Fatal(err)
	}
	defer a.ReleaseGroup()

	loaded, err := a.MoveChannelToGroup("moving-channel", "target-group", s)
	if err != nil {
		t.Fatal(err)
	}
	moved := false
	for _, c := range loaded.Channels {
		if c.ID == "moving-channel" {
			moved = c.GroupID == "target-group"
		}
	}
	if !moved {
		t.Fatal("channel metadata was not moved to target group")
	}
	if _, err = os.Stat(channelPath(dir, "target-group", "moving-channel")); err != nil {
		t.Fatalf("target channel folder missing: %v", err)
	}
	if _, err = os.Stat(channelPath(dir, s.Groups[0].ID, "moving-channel")); !os.IsNotExist(err) {
		t.Fatalf("source channel folder still exists: %v", err)
	}
}

func TestMoveChannelToGroupRejectsLockedTarget(t *testing.T) {
	dir := t.TempDir()
	s := channelMoveStore()
	if err := writeFolderStore(dir, s, ""); err != nil {
		t.Fatal(err)
	}
	a := &App{dir: dir, store: s}
	if err := os.MkdirAll(filepath.Join(dir, "locks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.AcquireGroup(s.Groups[0].ID); err != nil {
		t.Fatal(err)
	}
	defer a.ReleaseGroup()
	if err := os.WriteFile(a.lockPath("target-group"), []byte(strconv.Itoa(os.Getpid())), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(a.lockPath("target-group"))

	_, err := a.MoveChannelToGroup("moving-channel", "target-group", s)
	if err == nil || !strings.Contains(err.Error(), "다른 창") {
		t.Fatalf("expected locked-group rejection, got %v", err)
	}
	if _, statErr := os.Stat(channelPath(dir, s.Groups[0].ID, "moving-channel")); statErr != nil {
		t.Fatalf("source channel changed despite rejection: %v", statErr)
	}
}
