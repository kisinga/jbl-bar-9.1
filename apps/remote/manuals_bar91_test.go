package main

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBar91DeclaresItsManuals(t *testing.T) {
	f := bar91Backend{&remote{}}.Features()
	if f.Manuals["en"] == "" {
		t.Fatal("no English manual declared")
	}
	for lang, link := range f.Manuals {
		u, e := url.Parse(link)
		if len(lang) != 2 || e != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsAny(link, " '\"<>") ||
			!strings.HasSuffix(u.Path, "_"+strings.ToUpper(lang)+".pdf") || !strings.Contains(u.Path, "Bar 9.1") {
			t.Errorf("%s: %q is not an https Bar 9.1 manual in that language", lang, link)
		}
	}
}

// JBL can replace a PDF, which changes its link. Run on demand, with network:
//
//	JBL_CHECK_MANUALS=1 go test -run TestBar91ManualsOnline .
func TestBar91ManualsOnline(t *testing.T) {
	if os.Getenv("JBL_CHECK_MANUALS") == "" {
		t.Skip("set JBL_CHECK_MANUALS=1 to check the manual links online")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for lang, link := range bar91Manuals {
		res, err := client.Get(link)
		if err != nil {
			t.Errorf("%s: %v", lang, err)
			continue
		}
		head := make([]byte, 5)
		_, err = io.ReadFull(res.Body, head)
		res.Body.Close()
		if res.StatusCode != 200 || err != nil || string(head) != "%PDF-" {
			t.Errorf("%s: %s answered %d, not a PDF (%q)", lang, link, res.StatusCode, head)
		}
	}
}
