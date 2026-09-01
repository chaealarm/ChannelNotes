package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	htmlnode "golang.org/x/net/html"
)

type SearchResult struct {
	GroupID      string `json:"groupId"`
	GroupName    string `json:"groupName"`
	ChannelID    string `json:"channelId"`
	ChannelName  string `json:"channelName"`
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	NoteID       string `json:"noteId"`
	NoteName     string `json:"noteName"`
	Snippet      string `json:"snippet"`
	Matches      int    `json:"matches"`
}
type ReplaceResult struct {
	Replacements  int      `json:"replacements"`
	Files         int      `json:"files"`
	SkippedGroups []string `json:"skippedGroups"`
}

func scopeMatch(scope string, c Channel, cat Category, n Note, gid, cid, catid, nid string) bool {
	switch scope {
	case "note":
		return n.ID == nid
	case "category":
		return cat.ID == catid
	case "channel":
		return c.ID == cid
	case "group":
		return c.GroupID == gid
	default:
		return true
	}
}
func plainHTML(content string) string {
	doc, err := htmlnode.Parse(strings.NewReader("<html><body>" + content + "</body></html>"))
	if err != nil {
		return content
	}
	var b strings.Builder
	var walk func(*htmlnode.Node)
	walk = func(n *htmlnode.Node) {
		if n.Type == htmlnode.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return strings.Join(strings.FieldsFunc(b.String(), unicode.IsSpace), " ")
}
func countFold(s, q string) int {
	if q == "" {
		return 0
	}
	return strings.Count(strings.ToLower(s), strings.ToLower(q))
}
func shortSnippet(s string) string {
	r := []rune(s)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return s
}

func (a *App) SearchNotes(query, scope, gid, cid, catid, nid string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	a.mu.Lock()
	s, err := loadFolderStore(a.dir)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	gn := map[string]string{}
	for _, g := range s.Groups {
		gn[g.ID] = g.Name
	}
	out := []SearchResult{}
	for _, c := range s.Channels {
		for _, cat := range c.Categories {
			for _, n := range cat.Notes {
				if !scopeMatch(scope, c, cat, n, gid, cid, catid, nid) {
					continue
				}
				content, e := loadNoteFolder(a.dir, s, n.ID)
				if e != nil {
					continue
				}
				plain := plainHTML(content)
				hits := countFold(plain, query)
				if hits > 0 {
					out = append(out, SearchResult{GroupID: c.GroupID, GroupName: gn[c.GroupID], ChannelID: c.ID, ChannelName: c.Name, CategoryID: cat.ID, CategoryName: cat.Name, NoteID: n.ID, NoteName: n.Name, Snippet: shortSnippet(plain), Matches: hits})
				}
			}
		}
	}
	return out, nil
}

func replaceFold(s, q, repl string) (string, int) {
	if q == "" {
		return s, 0
	}
	lower, qLower := strings.ToLower(s), strings.ToLower(q)
	var b strings.Builder
	count := 0
	for {
		idx := strings.Index(lower, qLower)
		if idx < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:idx])
		b.WriteString(repl)
		s = s[idx+len(q):]
		lower = lower[idx+len(q):]
		count++
	}
	return b.String(), count
}
func replaceHTMLText(content, q, repl string) (string, int, error) {
	doc, err := htmlnode.Parse(strings.NewReader("<html><body>" + content + "</body></html>"))
	if err != nil {
		return "", 0, err
	}
	count := 0
	var body *htmlnode.Node
	var walk func(*htmlnode.Node)
	walk = func(n *htmlnode.Node) {
		if n.Type == htmlnode.ElementNode && n.Data == "body" {
			body = n
		}
		if n.Type == htmlnode.TextNode {
			var c int
			n.Data, c = replaceFold(n.Data, q, repl)
			count += c
		}
		for x := n.FirstChild; x != nil; x = x.NextSibling {
			walk(x)
		}
	}
	walk(doc)
	if body == nil {
		return "", 0, errors.New("본문을 읽을 수 없습니다")
	}
	var out bytes.Buffer
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		if err = htmlnode.Render(&out, c); err != nil {
			return "", 0, err
		}
	}
	return out.String(), count, nil
}

func (a *App) ReplaceNotes(find, repl, scope, gid, cid, catid, nid string) (ReplaceResult, error) {
	find = strings.TrimSpace(find)
	if find == "" {
		return ReplaceResult{}, errors.New("찾을 내용을 입력하세요")
	}
	a.mu.Lock()
	s, err := loadFolderStore(a.dir)
	own := a.lockGroup
	a.mu.Unlock()
	if err != nil {
		return ReplaceResult{}, err
	}
	result := ReplaceResult{}
	allowed := map[string]bool{own: true}
	releases := []func(){}
	defer func() {
		for _, r := range releases {
			r()
		}
	}()
	targetGroups := map[string]bool{}
	for _, c := range s.Channels {
		for _, cat := range c.Categories {
			for _, n := range cat.Notes {
				if scopeMatch(scope, c, cat, n, gid, cid, catid, nid) {
					targetGroups[c.GroupID] = true
				}
			}
		}
	}
	for target := range targetGroups {
		if target == own {
			continue
		}
		release, e := a.temporaryGroupLock(target)
		if e != nil {
			result.SkippedGroups = append(result.SkippedGroups, target)
			continue
		}
		allowed[target] = true
		releases = append(releases, release)
	}
	for _, c := range s.Channels {
		if !allowed[c.GroupID] {
			continue
		}
		for _, cat := range c.Categories {
			for _, n := range cat.Notes {
				if !scopeMatch(scope, c, cat, n, gid, cid, catid, nid) {
					continue
				}
				path := filepath.Join(notePath(a.dir, c.GroupID, c.ID, cat.ID, n.ID), "content.html")
				b, e := os.ReadFile(path)
				if e != nil {
					continue
				}
				changed, count, e := replaceHTMLText(string(b), find, repl)
				if e != nil {
					return result, e
				}
				if count > 0 {
					tmp := path + ".tmp"
					if e = os.WriteFile(tmp, []byte(changed), 0644); e != nil {
						return result, e
					}
					if e = os.Rename(tmp, path); e != nil {
						return result, e
					}
					result.Files++
					result.Replacements += count
				}
			}
		}
	}
	return result, nil
}
func (a *App) temporaryGroupLock(gid string) (func(), error) {
	path := a.lockPath(gid)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	if _, err = f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return func() { f.Close(); _ = os.Remove(path) }, nil
}
func contains(items []string, v string) bool {
	for _, x := range items {
		if x == v {
			return true
		}
	}
	return false
}
