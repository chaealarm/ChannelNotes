package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	htmlnode "golang.org/x/net/html"
	win "golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type Note struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	TitleLinked   bool   `json:"titleLinked"`
	Content       string `json:"content"`
	ContentLoaded bool   `json:"contentLoaded,omitempty"`
	UpdatedAt     string `json:"updatedAt"`
}
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Notes []Note `json:"notes"`
}
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Channel struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Image      string     `json:"image"`
	GroupID    string     `json:"groupId"`
	Notes      []Note     `json:"notes,omitempty"`
	Categories []Category `json:"categories"`
}
type Store struct {
	Groups          []Group   `json:"groups"`
	Channels        []Channel `json:"channels"`
	LastGroupID     string    `json:"lastGroupId"`
	LastChannelID   string    `json:"lastChannelId"`
	LastCategoryID  string    `json:"lastCategoryId"`
	LastNoteID      string    `json:"lastNoteId"`
	Theme           string    `json:"theme"`
	ShowGroupPopup  bool      `json:"showGroupPopup"`
	SettingsVersion int       `json:"settingsVersion"`
}
type ImageData struct {
	Name    string `json:"name"`
	DataURL string `json:"dataUrl"`
}
type GroupBundle struct {
	Group    Group     `json:"group"`
	Channels []Channel `json:"channels"`
}

type App struct {
	ctx        context.Context
	mu         sync.Mutex
	dir        string
	store      Store
	lockFile   *os.File
	lockGroup  string
	closeReady bool
}

func NewApp() *App { return &App{} }
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	base, _ := os.UserConfigDir()
	a.dir = filepath.Join(base, "ChannelNotes")
	_ = os.MkdirAll(a.dir, 0755)
	_ = os.MkdirAll(filepath.Join(a.dir, "locks"), 0755)
	if err := a.load(); err != nil {
		a.store = defaultStore()
		_ = a.persist()
	} else {
		a.normalize()
	}
	a.stripContents()
}
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closeReady {
		runtime.EventsEmit(ctx, "app:before-close")
		return true
	}
	a.releaseGroupUnlocked()
	return false
}
func (a *App) FinishClose() { a.mu.Lock(); a.closeReady = true; a.mu.Unlock(); runtime.Quit(a.ctx) }
func defaultStore() Store {
	gid, cid, catid, nid := newID(), newID(), newID(), newID()
	return Store{Groups: []Group{{ID: gid, Name: "기본 그룹"}}, Channels: []Channel{{ID: cid, Name: "내 채널", GroupID: gid, Categories: []Category{{ID: catid, Name: "메모장", Notes: []Note{{ID: nid, Title: "새 메모", Name: "새 메모", TitleLinked: true, Content: "<p>여기에 내용을 입력하세요.</p>", ContentLoaded: true}}}}}}, LastGroupID: gid, LastChannelID: cid, LastCategoryID: catid, LastNoteID: nid, Theme: "dark", ShowGroupPopup: true, SettingsVersion: 1}
}

func (a *App) normalize() {
	if a.store.SettingsVersion < 1 {
		a.store.ShowGroupPopup = true
		a.store.SettingsVersion = 1
	}
	if a.store.Theme == "" {
		a.store.Theme = "dark"
	}
	if len(a.store.Groups) == 0 {
		a.store.Groups = []Group{{ID: newID(), Name: "기본 그룹"}}
	}
	if a.store.LastGroupID == "" {
		a.store.LastGroupID = a.store.Groups[0].ID
	}
	for ci := range a.store.Channels {
		c := &a.store.Channels[ci]
		if c.GroupID == "" {
			c.GroupID = a.store.Groups[0].ID
		}
		if len(c.Categories) == 0 {
			c.Categories = []Category{{ID: newID(), Name: "메모장", Notes: c.Notes}}
			c.Notes = nil
		}
		for gi := range c.Categories {
			for ni := range c.Categories[gi].Notes {
				n := &c.Categories[gi].Notes[ni]
				if n.Name == "" {
					n.Name = n.Title
					n.TitleLinked = true
				}
			}
		}
	}
	if a.store.LastCategoryID == "" && len(a.store.Channels) > 0 && len(a.store.Channels[0].Categories) > 0 {
		a.store.LastCategoryID = a.store.Channels[0].Categories[0].ID
	}
}
func (a *App) stripContents() {
	for ci := range a.store.Channels {
		for gi := range a.store.Channels[ci].Categories {
			for ni := range a.store.Channels[ci].Categories[gi].Notes {
				n := &a.store.Channels[ci].Categories[gi].Notes[ni]
				n.Content = ""
				n.ContentLoaded = false
			}
		}
	}
}
func noteContentMap(s Store) map[string]string {
	m := map[string]string{}
	for _, c := range s.Channels {
		for _, g := range c.Categories {
			for _, n := range g.Notes {
				m[n.ID] = n.Content
			}
		}
	}
	return m
}
func hydrateUnloaded(s *Store, disk Store) {
	m := noteContentMap(disk)
	for ci := range s.Channels {
		for gi := range s.Channels[ci].Categories {
			for ni := range s.Channels[ci].Categories[gi].Notes {
				n := &s.Channels[ci].Categories[gi].Notes[ni]
				if !n.ContentLoaded {
					n.Content = m[n.ID]
				}
			}
		}
	}
}
func newID() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }
func (a *App) load() error {
	s, err := loadFolderStore(a.dir)
	a.store = s
	return err
}
func (a *App) persist() error { a.mu.Lock(); defer a.mu.Unlock(); return a.persistUnlocked() }
func (a *App) persistUnlocked() error {
	return writeFolderStore(a.dir, a.store, a.lockGroup)
}
func (a *App) GetStore() Store { a.mu.Lock(); defer a.mu.Unlock(); return a.store }
func (a *App) ReloadStore() (Store, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, err := loadFolderStore(a.dir)
	if err != nil {
		return Store{}, err
	}
	a.store = s
	a.normalize()
	a.stripContents()
	return a.store, nil
}
func (a *App) LoadNoteContent(noteID string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return loadNoteFolder(a.dir, a.store, noteID)
}
func processAlive(pid int) bool {
	h, err := win.OpenProcess(win.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	win.CloseHandle(h)
	return true
}
func (a *App) lockPath(gid string) string { return filepath.Join(a.dir, "locks", gid+".lock") }
func (a *App) releaseGroupUnlocked() {
	if a.lockFile != nil {
		a.lockFile.Close()
		_ = os.Remove(a.lockPath(a.lockGroup))
		a.lockFile = nil
		a.lockGroup = ""
	}
}
func (a *App) AcquireGroup(gid string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if gid == a.lockGroup {
		return nil
	}
	path := a.lockPath(gid)
	try := func() (*os.File, error) { return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644) }
	f, err := try()
	if err != nil {
		b, _ := os.ReadFile(path)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 && processAlive(pid) {
			return errors.New("다른 창에서 사용 중인 그룹입니다")
		}
		_ = os.Remove(path)
		f, err = try()
	}
	if err != nil {
		return err
	}
	if _, err = f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	_ = f.Sync()
	a.releaseGroupUnlocked()
	a.lockFile = f
	a.lockGroup = gid
	return nil
}
func (a *App) ReleaseGroup() { a.mu.Lock(); defer a.mu.Unlock(); a.releaseGroupUnlocked() }
func (a *App) LockedGroups() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	entries, _ := os.ReadDir(filepath.Join(a.dir, "locks"))
	result := []string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".lock" {
			continue
		}
		gid := strings.TrimSuffix(e.Name(), ".lock")
		if gid == a.lockGroup {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(a.dir, "locks", e.Name()))
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 && processAlive(pid) {
			result = append(result, gid)
		} else {
			_ = os.Remove(filepath.Join(a.dir, "locks", e.Name()))
		}
	}
	return result
}
func (a *App) SaveStore(s Store) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.store = s
	now := time.Now()
	for ci := range a.store.Channels {
		for gi := range a.store.Channels[ci].Categories {
			for ni := range a.store.Channels[ci].Categories[gi].Notes {
				if a.store.Channels[ci].Categories[gi].Notes[ni].ID == s.LastNoteID {
					a.store.Channels[ci].Categories[gi].Notes[ni].UpdatedAt = now.Format(time.RFC3339)
				}
			}
		}
	}
	if err := a.persistUnlocked(); err != nil {
		return "", err
	}
	a.stripContents()
	return now.Format("2006-01-02 15:04:05"), nil
}
func (a *App) SelectImages() ([]ImageData, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "이미지 삽입", Filters: []runtime.FileFilter{{DisplayName: "이미지", Pattern: "*.jpg;*.jpeg;*.png;*.webp;*.gif"}}})
	if err != nil {
		return nil, err
	}
	result := make([]ImageData, 0, len(paths))
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
		if ext == "jpg" {
			ext = "jpeg"
		}
		result = append(result, ImageData{Name: filepath.Base(p), DataURL: "data:image/" + ext + ";base64," + base64.StdEncoding.EncodeToString(b)})
	}
	return result, nil
}
func (a *App) BackupAll() (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "전체 메모 백업", DefaultFilename: "channel-notes-backup-" + time.Now().Format("20060102-150405") + ".zip", Filters: []runtime.FileFilter{{DisplayName: "ZIP 백업", Pattern: "*.zip"}}})
	if err != nil || path == "" {
		return "", err
	}
	return path, zipFolder(dataRoot(a.dir), path)
}
func (a *App) BackupGroup(groupID string) (string, error) {
	a.mu.Lock()
	latest, err := loadFolderStore(a.dir)
	a.mu.Unlock()
	if err != nil {
		return "", err
	}
	var bundle GroupBundle
	found := false
	for _, g := range latest.Groups {
		if g.ID == groupID {
			bundle.Group = g
			found = true
			break
		}
	}
	if !found {
		return "", errors.New("그룹을 찾을 수 없습니다")
	}
	for _, c := range latest.Channels {
		if c.GroupID == groupID {
			for gi := range c.Categories {
				for ni := range c.Categories[gi].Notes {
					content, e := loadNoteFolder(a.dir, latest, c.Categories[gi].Notes[ni].ID)
					if e == nil {
						c.Categories[gi].Notes[ni].Content = content
						c.Categories[gi].Notes[ni].ContentLoaded = true
					}
				}
			}
			bundle.Channels = append(bundle.Channels, c)
		}
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "그룹 백업", DefaultFilename: "channel-notes-group-" + time.Now().Format("20060102-150405") + ".zip", Filters: []runtime.FileFilter{{DisplayName: "그룹 ZIP 백업", Pattern: "*.zip"}}})
	if err != nil || path == "" {
		return "", err
	}
	zf, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer zf.Close()
	zw := zip.NewWriter(zf)
	defer zw.Close()
	w, err := zw.Create("group.json")
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}
	_, err = w.Write(data)
	return path, err
}
func (a *App) RestoreGroup() (GroupBundle, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "그룹 복원", Filters: []runtime.FileFilter{{DisplayName: "그룹 ZIP 백업", Pattern: "*.zip"}}})
	if err != nil || path == "" {
		return GroupBundle{}, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return GroupBundle{}, err
	}
	defer zr.Close()
	var bundle GroupBundle
	found := false
	for _, f := range zr.File {
		if f.Name != "group.json" {
			continue
		}
		r, e := f.Open()
		if e != nil {
			return GroupBundle{}, e
		}
		b, e := io.ReadAll(io.LimitReader(r, 100<<20))
		r.Close()
		if e != nil {
			return GroupBundle{}, e
		}
		if e = json.Unmarshal(b, &bundle); e != nil {
			return GroupBundle{}, e
		}
		found = true
		break
	}
	if !found {
		return GroupBundle{}, errors.New("올바른 그룹 백업이 아닙니다")
	}
	old := bundle.Group.ID
	bundle.Group.ID = newID()
	bundle.Group.Name += " (복원)"
	for ci := range bundle.Channels {
		c := &bundle.Channels[ci]
		c.ID = newID()
		c.GroupID = bundle.Group.ID
		for gi := range c.Categories {
			c.Categories[gi].ID = newID()
			for ni := range c.Categories[gi].Notes {
				c.Categories[gi].Notes[ni].ID = newID()
				c.Categories[gi].Notes[ni].ContentLoaded = true
			}
		}
	}
	_ = old
	return bundle, nil
}
func (a *App) RestoreAll() (Store, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "백업 복원", Filters: []runtime.FileFilter{{DisplayName: "ZIP 백업", Pattern: "*.zip"}}})
	if err != nil || path == "" {
		return Store{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, err := restoreFolderZip(a.dir, path)
	if err != nil {
		return Store{}, err
	}
	a.store = s
	a.normalize()
	a.stripContents()
	return a.store, nil
}
func (a *App) ExportNote(title, content string) (string, error) {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\\/:*?\"<>|`, r) {
			return '-'
		}
		return r
	}, title)
	if strings.TrimSpace(name) == "" {
		name = "memo"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "현재 메모 내보내기", DefaultFilename: name + ".html", Filters: []runtime.FileFilter{{DisplayName: "HTML 문서", Pattern: "*.html"}}})
	if err != nil || path == "" {
		return "", err
	}
	doc := "<!doctype html><html lang=\"ko\"><meta charset=\"utf-8\"><title>" + htmlEscape(title) + "</title><style>body{font-family:'Malgun Gothic',sans-serif;font-size:10pt;max-width:900px;margin:40px auto;line-height:1.65}h1{font-size:16pt}img{max-width:100%}</style><body>" + content + "</body></html>"
	return path, os.WriteFile(path, []byte(doc), 0644)
}

func (a *App) ImportNote() (Note, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "메모 불러오기", Filters: []runtime.FileFilter{{DisplayName: "HTML 문서", Pattern: "*.html;*.htm"}}})
	if err != nil || path == "" {
		return Note{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Note{}, err
	}
	doc, err := htmlnode.Parse(bytes.NewReader(b))
	if err != nil {
		return Note{}, err
	}
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var body *htmlnode.Node
	var walk func(*htmlnode.Node)
	walk = func(n *htmlnode.Node) {
		if n.Type == htmlnode.ElementNode && n.Data == "title" && n.FirstChild != nil && strings.TrimSpace(n.FirstChild.Data) != "" {
			title = strings.TrimSpace(n.FirstChild.Data)
		}
		if n.Type == htmlnode.ElementNode && n.Data == "body" {
			body = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if body == nil {
		return Note{}, errors.New("HTML 본문을 찾을 수 없습니다")
	}
	var clean func(*htmlnode.Node)
	clean = func(n *htmlnode.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == htmlnode.ElementNode && (c.Data == "script" || c.Data == "iframe" || c.Data == "object") {
				n.RemoveChild(c)
			} else {
				if c.Type == htmlnode.ElementNode {
					attrs := c.Attr[:0]
					for _, at := range c.Attr {
						key := strings.ToLower(at.Key)
						if !strings.HasPrefix(key, "on") && key != "srcdoc" {
							attrs = append(attrs, at)
						}
					}
					c.Attr = attrs
				}
				clean(c)
			}
			c = next
		}
	}
	clean(body)
	var out bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if err = htmlnode.Render(&out, c); err != nil {
			return Note{}, err
		}
	}
	return Note{ID: newID(), Title: title, Name: title, TitleLinked: true, Content: out.String(), ContentLoaded: true}, nil
}

func (a *App) SystemFonts() []string {
	seen := map[string]bool{"맑은 고딕": true}
	result := []string{"맑은 고딕"}
	keys := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	paths := []string{`SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`, `SOFTWARE\Microsoft\Windows\CurrentVersion\Fonts`}
	for _, root := range keys {
		for _, path := range paths {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			names, _ := k.ReadValueNames(0)
			for _, raw := range names {
				name := strings.TrimSpace(strings.NewReplacer("(TrueType)", "", "(OpenType)", "", "(All res)", "").Replace(raw))
				if name != "" && !seen[name] {
					seen[name] = true
					result = append(result, name)
				}
			}
			k.Close()
		}
	}
	return result
}
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
