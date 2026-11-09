package gitinfo

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kislow/muxrail/internal/testutil"
)

func TestStats(t *testing.T) {
	tmp := testutil.IsolatedTmp(t)
	work := clone(t)
	plain := t.TempDir()
	missing := filepath.Join(plain, "gone")
	ctx := context.Background()

	got, err := Stats(ctx, []string{work, plain, missing})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[work] == nil || got[work].Branch != "main" {
		t.Fatalf("got %+v, want only the repository", got)
	}

	write(t, filepath.Join(work, "f.txt"), lines(3)+"dirty\n")
	got, err = Stats(ctx, []string{work})
	if err != nil {
		t.Fatal(err)
	}
	if got[work].Dirty != (Lines{}) {
		t.Errorf("fresh cache entry must be served as is, got %+v", got[work].Dirty)
	}

	raw, err := os.ReadFile(filepath.Join(tmp, cacheName))
	if err != nil {
		t.Fatal(err)
	}
	var cache map[string]struct {
		Info *struct{ Committed, Dirty [2]int }
	}
	if err := json.Unmarshal(raw, &cache); err != nil {
		t.Fatalf("cache is not the shared format: %v\n%s", err, raw)
	}
	if _, ok := cache[plain]; !ok {
		t.Error("non-repositories are cached too, so they are not re-inspected every second")
	}
}

func TestStatsRefreshesStaleEntriesAndPrunesOldOnes(t *testing.T) {
	tmp := testutil.IsolatedTmp(t)
	work := clone(t)
	old := time.Now().Add(-10 * time.Minute)
	stale := time.Now().Add(-time.Minute)
	seed := map[string]entry{
		work:        {TS: unix(stale), Info: &Info{Branch: "outdated"}},
		"/long/ago": {TS: unix(old), Info: &Info{Branch: "x"}},
	}
	data, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(tmp, cacheName), string(data))

	got, err := Stats(context.Background(), []string{work})
	if err != nil {
		t.Fatal(err)
	}
	if got[work].Branch != "main" {
		t.Errorf("stale entry not refreshed: %+v", got[work])
	}
	raw, err := os.ReadFile(filepath.Join(tmp, cacheName))
	if err != nil {
		t.Fatal(err)
	}
	var after map[string]entry
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatal(err)
	}
	if _, ok := after["/long/ago"]; ok {
		t.Error("entries older than five minutes should be pruned")
	}
}
