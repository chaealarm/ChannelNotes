package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type settingsFile struct {
	LastGroupID     string `json:"lastGroupId"`
	LastChannelID   string `json:"lastChannelId"`
	LastCategoryID  string `json:"lastCategoryId"`
	LastNoteID      string `json:"lastNoteId"`
	Theme           string `json:"theme"`
	ShowGroupPopup  bool   `json:"showGroupPopup"`
	SettingsVersion int    `json:"settingsVersion"`
}

func validID(id string) bool { return id != "" && !strings.ContainsAny(id, `\/:*?"<>|.`) }
func atomicJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func dataRoot(dir string) string       { return filepath.Join(dir, "data") }
func groupPath(dir, gid string) string { return filepath.Join(dataRoot(dir), "groups", gid) }
func channelPath(dir, gid, cid string) string {
	return filepath.Join(groupPath(dir, gid), "channels", cid)
}
func categoryPath(dir, gid, cid, catid string) string {
	return filepath.Join(channelPath(dir, gid, cid), "categories", catid)
}
func notePath(dir, gid, cid, catid, nid string) string {
	return filepath.Join(categoryPath(dir, gid, cid, catid), "notes", nid)
}

func loadFolderStore(dir string) (Store, error) {
	var s Store
	if err := readJSON(filepath.Join(dataRoot(dir), "settings.json"), &s); err != nil {
		return s, err
	}
	groupsDir := filepath.Join(dataRoot(dir), "groups")
	ges, err := os.ReadDir(groupsDir)
	if err != nil {
		return s, err
	}
	for _, ge := range ges {
		if !ge.IsDir() {
			continue
		}
		var g Group
		if readJSON(filepath.Join(groupsDir, ge.Name(), "group.json"), &g) != nil {
			continue
		}
		s.Groups = append(s.Groups, g)
		ces, _ := os.ReadDir(filepath.Join(groupsDir, ge.Name(), "channels"))
		for _, ce := range ces {
			if !ce.IsDir() {
				continue
			}
			var c Channel
			if readJSON(filepath.Join(groupsDir, ge.Name(), "channels", ce.Name(), "channel.json"), &c) != nil {
				continue
			}
			cats, _ := os.ReadDir(filepath.Join(groupsDir, ge.Name(), "channels", ce.Name(), "categories"))
			for _, cae := range cats {
				if !cae.IsDir() {
					continue
				}
				var cat Category
				if readJSON(filepath.Join(groupsDir, ge.Name(), "channels", ce.Name(), "categories", cae.Name(), "category.json"), &cat) != nil {
					continue
				}
				nes, _ := os.ReadDir(filepath.Join(groupsDir, ge.Name(), "channels", ce.Name(), "categories", cae.Name(), "notes"))
				for _, ne := range nes {
					if !ne.IsDir() {
						continue
					}
					var n Note
					if readJSON(filepath.Join(groupsDir, ge.Name(), "channels", ce.Name(), "categories", cae.Name(), "notes", ne.Name(), "meta.json"), &n) != nil {
						continue
					}
					n.Content = ""
					n.ContentLoaded = false
					cat.Notes = append(cat.Notes, n)
				}
				c.Categories = append(c.Categories, cat)
			}
			s.Channels = append(s.Channels, c)
		}
	}
	if len(s.Groups) == 0 {
		return s, errors.New("저장된 그룹이 없습니다")
	}
	return s, nil
}

func writeFolderStore(dir string, s Store, locked string) error {
	settings := s
	settings.Groups = nil
	settings.Channels = nil
	if err := atomicJSON(filepath.Join(dataRoot(dir), "settings.json"), settings); err != nil {
		return err
	}
	for _, g := range s.Groups {
		gp := groupPath(dir, g.ID)
		_, statErr := os.Stat(gp)
		if locked != "" && g.ID != locked && statErr == nil {
			continue
		}
		if err := atomicJSON(filepath.Join(gp, "group.json"), g); err != nil {
			return err
		}
		keepChannels := map[string]bool{}
		for _, c := range s.Channels {
			if c.GroupID != g.ID {
				continue
			}
			keepChannels[c.ID] = true
			cp := channelPath(dir, g.ID, c.ID)
			cm := c
			cm.Categories = nil
			cm.Notes = nil
			if err := atomicJSON(filepath.Join(cp, "channel.json"), cm); err != nil {
				return err
			}
			keepCats := map[string]bool{}
			for _, cat := range c.Categories {
				keepCats[cat.ID] = true
				cap := categoryPath(dir, g.ID, c.ID, cat.ID)
				catm := cat
				catm.Notes = nil
				if err := atomicJSON(filepath.Join(cap, "category.json"), catm); err != nil {
					return err
				}
				keepNotes := map[string]bool{}
				for _, n := range cat.Notes {
					keepNotes[n.ID] = true
					np := notePath(dir, g.ID, c.ID, cat.ID, n.ID)
					nm := n
					nm.Content = ""
					nm.ContentLoaded = false
					if err := atomicJSON(filepath.Join(np, "meta.json"), nm); err != nil {
						return err
					}
					if n.ContentLoaded {
						if err := os.WriteFile(filepath.Join(np, "content.html"), []byte(n.Content), 0644); err != nil {
							return err
						}
					}
				}
				removeMissing(filepath.Join(cap, "notes"), keepNotes)
			}
			removeMissing(filepath.Join(cp, "categories"), keepCats)
		}
		removeMissing(filepath.Join(gp, "channels"), keepChannels)
	}
	if locked != "" {
		found := false
		for _, g := range s.Groups {
			if g.ID == locked {
				found = true
			}
		}
		if !found && validID(locked) {
			_ = os.RemoveAll(groupPath(dir, locked))
		}
	}
	return nil
}
func removeMissing(parent string, keep map[string]bool) {
	es, _ := os.ReadDir(parent)
	for _, e := range es {
		if e.IsDir() && !keep[e.Name()] && validID(e.Name()) {
			_ = os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}
func loadNoteFolder(dir string, s Store, nid string) (string, error) {
	for _, c := range s.Channels {
		for _, cat := range c.Categories {
			for _, n := range cat.Notes {
				if n.ID == nid {
					return stringOrErr(os.ReadFile(filepath.Join(notePath(dir, c.GroupID, c.ID, cat.ID, n.ID), "content.html")))
				}
			}
		}
	}
	return "", errors.New("메모를 찾을 수 없습니다")
}
func stringOrErr(b []byte, e error) (string, error) { return string(b), e }

func zipFolder(root, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		w, e := zw.Create(filepath.ToSlash(rel))
		if e != nil {
			return e
		}
		r, e := os.Open(path)
		if e != nil {
			return e
		}
		defer r.Close()
		_, e = io.Copy(w, r)
		return e
	})
}

func restoreFolderZip(dir, zipPath string) (Store, error) {
	staging, err := os.MkdirTemp(dir, "restore-")
	if err != nil {
		return Store{}, err
	}
	defer os.RemoveAll(staging)
	root := filepath.Join(staging, "data")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return Store{}, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return Store{}, errors.New("안전하지 않은 백업 경로입니다")
		}
		target := filepath.Join(root, clean)
		if f.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0755); err != nil {
				return Store{}, err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return Store{}, err
		}
		r, e := f.Open()
		if e != nil {
			return Store{}, e
		}
		w, e := os.Create(target)
		if e != nil {
			r.Close()
			return Store{}, e
		}
		_, e = io.Copy(w, io.LimitReader(r, 200<<20))
		r.Close()
		w.Close()
		if e != nil {
			return Store{}, e
		}
	}
	s, err := loadFolderStore(staging)
	if err != nil {
		return Store{}, errors.New("올바른 폴더형 Channel Notes 백업이 아닙니다")
	}
	current, old := dataRoot(dir), filepath.Join(staging, "old-data")
	if _, e := os.Stat(current); e == nil {
		if err = os.Rename(current, old); err != nil {
			return Store{}, err
		}
	}
	if err = os.Rename(root, current); err != nil {
		_ = os.Rename(old, current)
		return Store{}, err
	}
	return s, nil
}
