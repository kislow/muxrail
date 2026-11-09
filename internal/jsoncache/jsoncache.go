// Package jsoncache is the small file cache the sidebars and tab commands share, so the slow calls behind it run once per
// TTL for all of them instead of once per process.
package jsoncache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// Read decodes the cache file into v. It reports false when the file is missing, unreadable or older than ttl
// (ttl <= 0 accepts any age).
func Read(path string, ttl time.Duration, v any) bool {
	info, err := os.Stat(path)
	if err != nil || (ttl > 0 && time.Since(info.ModTime()) >= ttl) {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// Write replaces the cache file atomically: readers see the old or the new content, never half of it.
func Write(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("%w: encode cache", err)
	}
	tmp := fmt.Sprintf("%s.%d", path, os.Getpid())
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("%w: write cache", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Join(fmt.Errorf("%w: replace cache", err), os.Remove(tmp))
	}
	return nil
}
