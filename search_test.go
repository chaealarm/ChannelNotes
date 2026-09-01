package main

import (
	"strings"
	"testing"
)

func TestReplaceHTMLTextOnly(t *testing.T) {
	input := `<p title="찾기">찾기 <b>FIND</b></p><img alt="찾기" src="data:image/png;base64,abc">`
	out, count, err := replaceHTMLText(input, "find", "교체")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || !strings.Contains(out, "<b>교체</b>") {
		t.Fatalf("unexpected replacement: count=%d out=%s", count, out)
	}
	if !strings.Contains(out, `title="찾기"`) || !strings.Contains(out, `alt="찾기"`) {
		t.Fatalf("attributes were modified: %s", out)
	}
}

func TestPlainHTMLSearchIgnoresMarkup(t *testing.T) {
	plain := plainHTML(`<p>안녕하세요 <b>메모</b></p><img src="data:image/png;base64,SEARCH">`)
	if countFold(plain, "메모") != 1 {
		t.Fatalf("text not found: %s", plain)
	}
	if countFold(plain, "SEARCH") != 0 {
		t.Fatalf("image data leaked into search text: %s", plain)
	}
}
