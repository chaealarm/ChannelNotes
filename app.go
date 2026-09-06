package main

import (
	"archive/zip"
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
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
	Order         int    `json:"order"`
}
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
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
	Order      int        `json:"order"`
	Notes      []Note     `json:"notes,omitempty"`
	Categories []Category `json:"categories"`
}
type Store struct {
	Groups           []Group   `json:"groups"`
	Channels         []Channel `json:"channels"`
	LastGroupID      string    `json:"lastGroupId"`
	LastChannelID    string    `json:"lastChannelId"`
	LastCategoryID   string    `json:"lastCategoryId"`
	LastNoteID       string    `json:"lastNoteId"`
	Theme            string    `json:"theme"`
	ShowGroupPopup   bool      `json:"showGroupPopup"`
	PeriodicAutoSave bool      `json:"periodicAutoSave"`
	HideChannels     bool      `json:"hideChannels"`
	HideNotes        bool      `json:"hideNotes"`
	OpenNoteIDs      []string  `json:"openNoteIds"`
	ImageInsertWidth int       `json:"imageInsertWidth"`
	SettingsVersion  int       `json:"settingsVersion"`
}
type ImageData struct {
	Name    string `json:"name"`
	DataURL string `json:"dataUrl"`
}
type GroupBundle struct {
	Group    Group     `json:"group"`
	Channels []Channel `json:"channels"`
}
type DetachedNote struct {
	Note             Note   `json:"note"`
	Theme            string `json:"theme"`
	ImageInsertWidth int    `json:"imageInsertWidth"`
}
type detachedWindow struct {
	window    *application.WebviewWindow
	createdAt time.Time
	closing   bool
}

type App struct {
	ctx        context.Context
	wails      *application.App
	mainWindow *application.WebviewWindow
	mu         sync.Mutex
	dir        string
	store      Store
	lockFile   *os.File
	lockGroup  string
	closeReady bool
	detached   map[string]*detachedWindow
}

func NewApp() *App { return &App{detached: map[string]*detachedWindow{}} }
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
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
	return nil
}
func (a *App) beforeMainClose(event *application.WindowEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closeReady {
		event.Cancel()
		a.wails.Event.Emit("app:before-close")
		return
	}
	a.releaseGroupUnlocked()
}
func (a *App) shutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = a.persistUnlocked()
	a.releaseGroupUnlocked()
}
func (a *App) FinishClose() {
	a.mu.Lock()
	a.closeReady = true
	a.mu.Unlock()
	a.wails.Quit()
}
func defaultStore() Store {
	gid, cid, catid, nid := newID(), newID(), newID(), newID()
	return Store{Groups: []Group{{ID: gid, Name: "기본 그룹"}}, Channels: []Channel{{ID: cid, Name: "내 채널", GroupID: gid, Categories: []Category{{ID: catid, Name: "메모장", Notes: []Note{{ID: nid, Title: "새 메모", Name: "새 메모", TitleLinked: true, Content: "<p>여기에 내용을 입력하세요.</p>", ContentLoaded: true}}}}}}, LastGroupID: gid, LastChannelID: cid, LastCategoryID: catid, LastNoteID: nid, Theme: "dark", ShowGroupPopup: true, PeriodicAutoSave: true, ImageInsertWidth: 100, SettingsVersion: 2}
}

func (a *App) normalize() {
	if a.store.SettingsVersion < 1 {
		a.store.ShowGroupPopup = true
		a.store.SettingsVersion = 1
	}
	if a.store.SettingsVersion < 2 {
		a.store.PeriodicAutoSave = true
		a.store.SettingsVersion = 2
	}
	if a.store.Theme == "" {
		a.store.Theme = "dark"
	}
	if a.store.ImageInsertWidth < 5 || a.store.ImageInsertWidth > 100 {
		a.store.ImageInsertWidth = 100
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

var fallbackIDCounter uint64

func newID() string {
	var random [16]byte
	if _, err := cryptorand.Read(random[:]); err == nil {
		return hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), atomic.AddUint64(&fallbackIDCounter, 1))
}
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

func (a *App) acquireAdditionalGroupUnlocked(gid string) (*os.File, error) {
	path := a.lockPath(gid)
	open := func() (*os.File, error) {
		return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	}
	f, err := open()
	if err != nil {
		b, _ := os.ReadFile(path)
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 && processAlive(pid) {
			return nil, errors.New("다른 창에서 사용 중인 그룹입니다")
		}
		_ = os.Remove(path)
		f, err = open()
	}
	if err != nil {
		return nil, err
	}
	if _, err = f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	_ = f.Sync()
	return f, nil
}

// MoveChannelToGroup moves the channel directory while holding both the
// currently edited group lock and a temporary destination lock. This keeps a
// second application process from modifying the destination between the UI's
// lock check and the actual move.
func (a *App) MoveChannelToGroup(channelID, targetGroupID string, s Store) (Store, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if targetGroupID == a.lockGroup {
		return s, errors.New("이미 현재 그룹에 있는 채널입니다")
	}
	var moved Channel
	found := false
	sourceCount := 0
	for _, c := range s.Channels {
		if c.GroupID == a.lockGroup {
			sourceCount++
		}
		if c.ID == channelID && c.GroupID == a.lockGroup {
			moved, found = c, true
		}
	}
	if !found {
		return s, errors.New("현재 그룹에서 채널을 찾을 수 없습니다")
	}
	if sourceCount <= 1 {
		return s, errors.New("그룹의 마지막 채널은 이동할 수 없습니다")
	}
	targetExists := false
	for _, g := range s.Groups {
		if g.ID == targetGroupID {
			targetExists = true
			break
		}
	}
	if !targetExists || !validID(targetGroupID) || !validID(channelID) {
		return s, errors.New("대상 그룹을 찾을 수 없습니다")
	}

	// First flush every pending edit to the source group.
	a.store = s
	if err := a.persistUnlocked(); err != nil {
		return s, err
	}
	targetLock, err := a.acquireAdditionalGroupUnlocked(targetGroupID)
	if err != nil {
		return s, err
	}
	defer func() {
		targetLock.Close()
		_ = os.Remove(a.lockPath(targetGroupID))
	}()

	sourcePath := channelPath(a.dir, a.lockGroup, channelID)
	targetPath := channelPath(a.dir, targetGroupID, channelID)
	if err = os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return s, err
	}
	if _, err = os.Stat(targetPath); err == nil {
		return s, errors.New("대상 그룹에 같은 채널 데이터가 이미 있습니다")
	}
	if err = os.Rename(sourcePath, targetPath); err != nil {
		return s, err
	}
	moved.GroupID = targetGroupID
	moved.Categories = nil
	moved.Notes = nil
	if err = atomicJSON(filepath.Join(targetPath, "channel.json"), moved); err != nil {
		_ = os.Rename(targetPath, sourcePath)
		return s, err
	}

	loaded, err := loadFolderStore(a.dir)
	if err != nil {
		return s, err
	}
	loaded.Theme = s.Theme
	loaded.ShowGroupPopup = s.ShowGroupPopup
	loaded.PeriodicAutoSave = s.PeriodicAutoSave
	loaded.HideChannels = s.HideChannels
	loaded.HideNotes = s.HideNotes
	loaded.OpenNoteIDs = s.OpenNoteIDs
	loaded.SettingsVersion = s.SettingsVersion
	loaded.LastGroupID = s.LastGroupID
	loaded.LastChannelID = s.LastChannelID
	loaded.LastCategoryID = s.LastCategoryID
	loaded.LastNoteID = s.LastNoteID
	repairLastSelection(&loaded)
	a.store = loaded
	if err = writeFolderStore(a.dir, a.store, targetGroupID); err != nil {
		return s, err
	}
	a.stripContents()
	return a.store, nil
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

func (a *App) detachedNoteUnlocked(noteID string) (DetachedNote, error) {
	for _, c := range a.store.Channels {
		if c.GroupID != a.lockGroup {
			continue
		}
		for _, cat := range c.Categories {
			for _, n := range cat.Notes {
				if n.ID == noteID {
					content, err := loadNoteFolder(a.dir, a.store, noteID)
					if err != nil {
						return DetachedNote{}, err
					}
					n.Content, n.ContentLoaded = content, true
					return DetachedNote{Note: n, Theme: a.store.Theme, ImageInsertWidth: a.store.ImageInsertWidth}, nil
				}
			}
		}
	}
	return DetachedNote{}, errors.New("현재 그룹에서 메모를 찾을 수 없습니다")
}

func (a *App) GetDetachedNote(noteID string) (DetachedNote, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.detachedNoteUnlocked(noteID)
}

func (a *App) SaveDetachedNote(noteID, title, content string) (string, error) {
	a.mu.Lock()
	now := time.Now()
	found := false
	for ci := range a.store.Channels {
		c := &a.store.Channels[ci]
		if c.GroupID != a.lockGroup {
			continue
		}
		for gi := range c.Categories {
			for ni := range c.Categories[gi].Notes {
				n := &c.Categories[gi].Notes[ni]
				if n.ID != noteID {
					continue
				}
				n.Title = strings.TrimSpace(title)
				if n.Title == "" {
					n.Title = "제목 없음"
				}
				if n.TitleLinked {
					n.Name = n.Title
				}
				n.Content, n.ContentLoaded, n.UpdatedAt = content, true, now.Format(time.RFC3339)
				if err := atomicJSON(filepath.Join(notePath(a.dir, c.GroupID, c.ID, c.Categories[gi].ID, n.ID), "meta.json"), *n); err != nil {
					a.mu.Unlock()
					return "", err
				}
				if err := os.WriteFile(filepath.Join(notePath(a.dir, c.GroupID, c.ID, c.Categories[gi].ID, n.ID), "content.html"), []byte(content), 0644); err != nil {
					a.mu.Unlock()
					return "", err
				}
				found = true
			}
		}
	}
	a.mu.Unlock()
	if !found {
		return "", errors.New("현재 그룹에서 메모를 찾을 수 없습니다")
	}
	a.wails.Event.Emit("note:updated", map[string]any{"noteId": noteID, "title": title, "content": content})
	return now.Format("2006-01-02 15:04:05"), nil
}
func (a *App) SetImageInsertWidth(width int) error {
	if width < 5 {
		width = 5
	}
	if width > 100 {
		width = 100
	}
	a.mu.Lock()
	a.store.ImageInsertWidth = width
	err := a.persistUnlocked()
	a.mu.Unlock()
	if err == nil {
		a.wails.Event.Emit("image:insert-width", width)
	}
	return err
}

func (a *App) OpenDetachedNote(noteID, title string) error {
	a.mu.Lock()
	if existing := a.detached[noteID]; existing != nil {
		w := existing.window
		a.mu.Unlock()
		w.Show()
		w.Focus()
		return nil
	}
	if _, err := a.detachedNoteUnlocked(noteID); err != nil {
		a.mu.Unlock()
		return err
	}
	a.mu.Unlock()

	x, y := a.mainWindow.Position()
	w, _ := a.mainWindow.Size()
	detached := a.wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "note-" + noteID, Title: title, Width: 760, Height: 680, MinWidth: 480, MinHeight: 360,
		URL: "/detached.html?note=" + url.QueryEscape(noteID), InitialPosition: application.WindowXY,
		X: x + w + 18, Y: y + 60, BackgroundColour: application.NewRGBA(30, 31, 34, 255), EnableFileDrop: true,
	})
	entry := &detachedWindow{window: detached, createdAt: time.Now()}
	a.mu.Lock()
	a.detached[noteID] = entry
	a.mu.Unlock()
	detached.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		a.mu.Lock()
		delete(a.detached, noteID)
		a.mu.Unlock()
		a.wails.Event.Emit("note:reattach", noteID)
	})
	detached.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		a.handleDroppedImages(noteID, event)
	})
	detached.OnWindowEvent(events.Windows.WindowEndMove, func(_ *application.WindowEvent) {
		a.mu.Lock()
		e := a.detached[noteID]
		a.mu.Unlock()
		if e == nil || time.Since(e.createdAt) < time.Second {
			return
		}
		dx, dy := detached.Position()
		dw, _ := detached.Size()
		mx, my := a.mainWindow.Position()
		mw, _ := a.mainWindow.Size()
		center := dx + dw/2
		if center >= mx && center <= mx+mw && dy >= my && dy <= my+130 {
			detached.Close()
			a.mainWindow.Show()
			a.mainWindow.Focus()
		}
	})
	return nil
}

func (a *App) ReattachDetachedNote(noteID string) {
	a.mu.Lock()
	entry := a.detached[noteID]
	a.mu.Unlock()
	if entry != nil {
		entry.window.Close()
	}
}
func (a *App) openFile(title, display, pattern string) (string, error) {
	return a.wails.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		Title: title, Filters: []application.FileFilter{{DisplayName: display, Pattern: pattern}},
	}).PromptForSingleSelection()
}
func (a *App) saveFile(title, filename, display, pattern string) (string, error) {
	return a.wails.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title: title, Filename: filename, Filters: []application.FileFilter{{DisplayName: display, Pattern: pattern}},
	}).PromptForSingleSelection()
}
func (a *App) SelectImages() ([]ImageData, error) {
	paths, err := a.wails.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{Title: "이미지 삽입", AllowsMultipleSelection: true, Filters: []application.FileFilter{{DisplayName: "이미지", Pattern: "*.jpg;*.jpeg;*.png;*.webp;*.gif"}}}).PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	return imageDataFromPaths(paths), nil
}
func imageDataFromPaths(paths []string) []ImageData {
	result := make([]ImageData, 0, len(paths))
	for _, p := range paths {
		ext := strings.ToLower(filepath.Ext(p))
		mime, ok := map[string]string{".jpg": "jpeg", ".jpeg": "jpeg", ".png": "png", ".webp": "webp", ".gif": "gif"}[ext]
		if !ok {
			continue
		}
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		result = append(result, ImageData{Name: filepath.Base(p), DataURL: "data:image/" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)})
	}
	return result
}
func (a *App) handleDroppedImages(target string, event *application.WindowEvent) {
	images := imageDataFromPaths(event.Context().DroppedFiles())
	if len(images) == 0 {
		return
	}
	details := event.Context().DropTargetDetails()
	payload := map[string]any{"target": target, "images": images}
	if details != nil {
		payload["x"], payload["y"] = details.X, details.Y
	}
	a.wails.Event.Emit("editor:images-dropped", payload)
}
func (a *App) BackupAll() (string, error) {
	path, err := a.saveFile("전체 메모 백업", "channel-notes-backup-"+time.Now().Format("20060102-150405")+".zip", "ZIP 백업", "*.zip")
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
					if e != nil {
						return "", fmt.Errorf("메모 %q 본문을 백업할 수 없습니다: %w", c.Categories[gi].Notes[ni].Name, e)
					}
					c.Categories[gi].Notes[ni].Content = content
					c.Categories[gi].Notes[ni].ContentLoaded = true
				}
			}
			bundle.Channels = append(bundle.Channels, c)
		}
	}
	path, err := a.saveFile("그룹 백업", "channel-notes-group-"+time.Now().Format("20060102-150405")+".zip", "그룹 ZIP 백업", "*.zip")
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
	path, err := a.openFile("그룹 복원", "그룹 ZIP 백업", "*.zip")
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
	bundle = prepareRestoredGroup(bundle)
	if err = a.persistRestoredGroup(bundle); err != nil {
		return GroupBundle{}, err
	}
	return bundle, nil
}
func prepareRestoredGroup(bundle GroupBundle) GroupBundle {
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
	return bundle
}
func (a *App) persistRestoredGroup(bundle GroupBundle) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	latest, err := loadFolderStore(a.dir)
	if err != nil {
		return err
	}
	for _, g := range latest.Groups {
		if g.ID == bundle.Group.ID {
			return errors.New("같은 식별자의 그룹이 이미 존재합니다")
		}
	}
	latest.Groups = append(latest.Groups, bundle.Group)
	latest.Channels = append(latest.Channels, bundle.Channels...)
	if err = writeFolderStore(a.dir, latest, bundle.Group.ID); err != nil {
		return fmt.Errorf("복원 그룹 저장 실패: %w", err)
	}
	a.store = latest
	a.stripContents()
	return nil
}
func (a *App) RestoreAll() (Store, error) {
	path, err := a.openFile("백업 복원", "ZIP 백업", "*.zip")
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
	path, err := a.saveFile("현재 메모 내보내기", name+".html", "HTML 문서", "*.html")
	if err != nil || path == "" {
		return "", err
	}
	doc := "<!doctype html><html lang=\"ko\"><meta charset=\"utf-8\"><title>" + htmlEscape(title) + "</title><style>body{font-family:'Malgun Gothic',sans-serif;font-size:10pt;max-width:900px;margin:40px auto;line-height:1.65}h1{font-size:16pt}img{max-width:100%}</style><body>" + content + "</body></html>"
	return path, os.WriteFile(path, []byte(doc), 0644)
}

func (a *App) ImportNote() (Note, error) {
	path, err := a.openFile("메모 불러오기", "HTML 문서", "*.html;*.htm")
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
