package utd

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestDebPackageName(t *testing.T) {
	cases := map[string]string{
		"/tmp/x/figlet_2.2.5-3+b2_amd64.deb":   "figlet",
		"nmap-common_7.95+dfsg-3_all.deb":      "nmap-common",
		"g++_4%3a13.2.0-5_amd64.deb":           "g++",
		"liblinear4_2.3.0+dfsg-5+b2_amd64.deb": "liblinear4",
		"/a/b/weird.deb":                       "weird",
		"python3-pip_24.0+dfsg-1_all.deb":      "python3-pip",
		"fd-find_10.2.0-1+b5_amd64.deb":        "fd-find",
		"nested/dir/foo_1.0_arm64.deb":         "foo",
		"trailing_underscore_name_1.0_all.deb": "trailing",
	}
	for in, want := range cases {
		if got := debPackageName(in); got != want {
			t.Errorf("debPackageName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"fd-find":       "fdfind",
		"fdfind":        "fdfind",
		"figlet-figlet": "figletfiglet",
		"g++":           "g",
		"7z":            "7z",
		"A.B_C":         "abc",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1 KiB",
		1536:       "1.5 KiB",
		1048576:    "1 MiB",
		1073741824: "1 GiB",
	}
	for in, want := range cases {
		if got := humanSize(in); got != want {
			t.Errorf("humanSize(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestLookupAlias(t *testing.T) {
	a, ok := LookupAlias("bat")
	if !ok {
		t.Fatal("expected an alias for bat")
	}
	if a.Package != "bat" || a.Binary != "batcat" {
		t.Fatalf("unexpected bat alias: %+v", a)
	}
	if _, ok := LookupAlias("this-command-does-not-exist"); ok {
		t.Fatal("did not expect an alias for an unknown command")
	}
}

// writeGzip writes content into a gzip compressed file and returns its path.
func writeGzip(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Contents-amd64.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	if _, err := zw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSearchContentsFile(t *testing.T) {
	content := "" +
		"bin/dig                                                net/bind9-dnsutils\n" +
		"usr/bin/dig                                            net/bind9-dnsutils\n" +
		"usr/bin/figlet                                         utils/figlet\n" +
		"usr/bin/rg                                             utils/ripgrep\n" +
		"usr/bin/rgrep                                          utils/grep\n" +
		"usr/share/doc/rg/changelog.gz                          utils/ripgrep\n" +
		"usr/sbin/zzz                                           admin/zzz\n"
	path := writeGzip(t, content)

	if got := searchContentsFile(path, "bind9"); len(got) != 0 {
		t.Errorf("unexpected match for bind9: %v", got)
	}
	if got := searchContentsFile(path, "rg"); len(got) != 1 || got[0] != "ripgrep" {
		t.Errorf("searchContentsFile(rg) = %v, want [ripgrep]", got)
	}
	if got := searchContentsFile(path, "figlet"); len(got) != 1 || got[0] != "figlet" {
		t.Errorf("searchContentsFile(figlet) = %v, want [figlet]", got)
	}
	if got := searchContentsFile(path, "dig"); len(got) != 1 || got[0] != "bind9-dnsutils" {
		t.Errorf("searchContentsFile(dig) = %v, want [bind9-dnsutils]", got)
	}
	if got := searchContentsFile(path, "absent"); len(got) != 0 {
		t.Errorf("searchContentsFile(absent) = %v, want none", got)
	}
}

func TestSearchContentsFileMultipleProviders(t *testing.T) {
	content := "usr/bin/link  utils/one,utils/two\n"
	path := writeGzip(t, content)
	got := searchContentsFile(path, "link")
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %v, want [one two]", got)
	}
}

// fakeManager is a PackageManager used to exercise the resolver without apt.
type fakeManager struct {
	existing      map[string]bool
	providers     map[string][]ProviderResult
	providerCalls int
}

func (f *fakeManager) Name() string                { return "fake" }
func (f *fakeManager) Available() bool             { return true }
func (f *fakeManager) PackageExists(p string) bool { return f.existing[p] }
func (f *fakeManager) IsInstalled(string) bool     { return false }
func (f *fakeManager) ResolveDependencies(p string) ([]string, error) {
	return []string{p}, nil
}
func (f *fakeManager) Download([]string, string) ([]string, error) { return nil, nil }
func (f *fakeManager) Extract(string, string) error                { return nil }
func (f *fakeManager) CommandProviders(cmd string) []ProviderResult {
	f.providerCalls++
	return f.providers[cmd]
}

// TestResolverPrimaryAvoidsIndexScan makes sure the cheap tier never triggers
// the (slow) index scan when a same-name package already answers.
func TestResolverPrimaryAvoidsIndexScan(t *testing.T) {
	mgr := &fakeManager{
		existing:  map[string]bool{"figlet": true},
		providers: map[string][]ProviderResult{"figlet": {{Package: "other", Source: "contents"}}},
	}
	r := NewResolver(mgr, NewLogger(LevelQuiet, os.Stderr, false))
	cands := r.Primary("figlet")
	if len(cands) != 1 || cands[0].Package != "figlet" {
		t.Fatalf("Primary(figlet) = %+v", cands)
	}
	if mgr.providerCalls != 0 {
		t.Fatalf("Primary triggered %d index scans, want 0", mgr.providerCalls)
	}
	if got := r.Extended("figlet"); len(got) != 1 || got[0].Package != "other" {
		t.Fatalf("Extended(figlet) = %+v", got)
	}
	if mgr.providerCalls != 1 {
		t.Fatalf("Extended made %d index scans, want 1", mgr.providerCalls)
	}
}

func TestResolverOrderAndDeduplication(t *testing.T) {
	mgr := &fakeManager{
		existing: map[string]bool{"figlet": true},
		providers: map[string][]ProviderResult{
			"figlet": {
				{Package: "figlet", Source: "contents"},
				{Package: "figlet-extra", Source: "contents"},
			},
		},
	}
	r := NewResolver(mgr, NewLogger(LevelQuiet, os.Stderr, false))
	cands := r.Candidates("figlet")
	if len(cands) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(cands), cands)
	}
	if cands[0].Package != "figlet" || cands[0].Source != "package of the same name" {
		t.Fatalf("unexpected first candidate: %+v", cands[0])
	}
	if cands[1].Package != "figlet-extra" {
		t.Fatalf("unexpected second candidate: %+v", cands[1])
	}
}

func TestResolverPrefersAlias(t *testing.T) {
	mgr := &fakeManager{
		existing:  map[string]bool{},
		providers: map[string][]ProviderResult{"bat": {{Package: "bacula-console-qt", Source: "contents"}}},
	}
	r := NewResolver(mgr, NewLogger(LevelQuiet, os.Stderr, false))
	cands := r.Candidates("bat")
	if len(cands) < 2 {
		t.Fatalf("got %d candidates, want at least 2", len(cands))
	}
	if cands[0].Package != "bat" || cands[0].Binary != "batcat" {
		t.Fatalf("alias should come first, got %+v", cands[0])
	}
}

func TestCacheFindBinaryAndIndex(t *testing.T) {
	root := t.TempDir()
	cache := NewCache(root, NewLogger(LevelQuiet, os.Stderr, false))
	if err := cache.Ensure(); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(cache.ExtractedDir("figlet"), "usr", "bin", "figlet-figlet")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, ok := cache.FindBinary("figlet-figlet"); !ok || got != bin {
		t.Fatalf("FindBinary = %q, %v", got, ok)
	}
	if got, ok := cache.FindFuzzyBinary("figlet", "figlet"); !ok || got != bin {
		t.Fatalf("FindFuzzyBinary = %q, %v", got, ok)
	}
	if _, ok := cache.FindBinary("does-not-exist"); ok {
		t.Fatal("FindBinary found a non existent binary")
	}

	cache.Remember("figlet", "figlet", "figlet-figlet", []string{"figlet"})
	idx := cache.IndexLoad()
	entry, ok := idx.Commands["figlet"]
	if !ok || entry.Package != "figlet" || entry.Binary != "figlet-figlet" {
		t.Fatalf("index round trip failed: %+v", idx)
	}

	if err := cache.RemovePackage("figlet"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.FindBinary("figlet-figlet"); ok {
		t.Fatal("binary still present after RemovePackage")
	}
	if _, ok := cache.IndexLoad().Commands["figlet"]; ok {
		t.Fatal("index entry still present after RemovePackage")
	}
}

func TestGlobEscape(t *testing.T) {
	if got := globEscape("g++"); got != "g++" {
		t.Errorf("globEscape(g++) = %q", got)
	}
	if got := globEscape("a*b"); got != `a\*b` {
		t.Errorf("globEscape(a*b) = %q", got)
	}
}
