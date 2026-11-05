package main

import (
	"net/url"
	"strings"
	"testing"
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
