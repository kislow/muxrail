package gitinfo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kislow/muxrail/internal/jsoncache"
)

const (
	statsTTL  = 5 * time.Second
	pruneAge  = 5 * time.Minute
	cacheName = "tmux-sidebar-git.json"
)

type entry struct {
	TS   float64 `json:"ts"`
	Info *Info   `json:"info"`
}

func unix(t time.Time) float64 { return float64(t.UnixNano()) / 1e9 }

// Stats returns Info for each of paths that is inside a repository. Results come from a cache file the sidebars
// share, so each path is inspected once per statsTTL however many sidebar processes ask.
func Stats(ctx context.Context, paths []string) (map[string]*Info, error) {
	file := filepath.Join(os.TempDir(), cacheName)
	cache := map[string]entry{}
	if !jsoncache.Read(file, 0, &cache) || cache == nil {
		cache = map[string]entry{}
	}
	now := time.Now()
	changed := false
	for _, p := range paths {
		if e, ok := cache[p]; ok && now.Sub(fromUnix(e.TS)) <= statsTTL {
			continue
		}
		cache[p] = entry{TS: unix(now), Info: inspect(ctx, p)}
		changed = true
	}
	if changed {
		for p, e := range cache {
			if now.Sub(fromUnix(e.TS)) >= pruneAge {
				delete(cache, p)
			}
		}
		if err := jsoncache.Write(file, cache); err != nil {
			return nil, fmt.Errorf("%w: git stats", err)
		}
	}
	found := map[string]*Info{}
	for _, p := range paths {
		if e := cache[p]; e.Info != nil {
			found[p] = e.Info
		}
	}
	return found, nil
}

func fromUnix(ts float64) time.Time {
	return time.Unix(0, int64(ts*1e9))
}

func inspect(ctx context.Context, path string) *Info {
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return nil
	}
	return Compute(ctx, path)
}
