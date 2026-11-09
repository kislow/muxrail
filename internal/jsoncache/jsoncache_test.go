package jsoncache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteThenRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	if err := Write(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	var got map[string]int
	if !Read(path, time.Minute, &got) || got["a"] != 1 {
		t.Errorf("read back %v", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

func TestReadRejectsStaleMissingAndCorruptFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	var v map[string]int
	if Read(path, time.Minute, &v) {
		t.Error("missing file must not read")
	}
	if err := Write(path, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if Read(path, time.Minute, &v) {
		t.Error("stale file must not read")
	}
	if !Read(path, 0, &v) {
		t.Error("ttl 0 accepts any age")
	}
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Read(path, 0, &v) {
		t.Error("corrupt file must not read")
	}
}

func TestWriteFailsInAMissingDirectory(t *testing.T) {
	if err := Write(filepath.Join(t.TempDir(), "no", "c.json"), 1); err == nil {
		t.Error("want an error")
	}
}
