// Package release resolves the newest published Android build from the
// project's GitHub releases, so shipping an APK does not also require
// redeploying the server with a new LATEST_VERSION_CODE.
package release

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VersionCodeFromTag mirrors the formula in .github/workflows/release.yml —
// the two must agree or clients compare against a code no APK ever carried.
// "v1.2.3" -> 10203. Returns 0 for anything that is not vMAJOR.MINOR.PATCH.
func VersionCodeFromTag(tag string) int {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(tag), "v"), ".")
	if len(parts) != 3 {
		return 0
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return 0
		}
		nums[i] = n
	}
	if nums[1] > 99 || nums[2] > 99 {
		return 0 // would collide with the next major/minor
	}
	return nums[0]*10000 + nums[1]*100 + nums[2]
}

// Tracker polls the GitHub releases API in the background. Reads never block
// on the network: callers get the last successful answer, or zero until the
// first poll lands.
type Tracker struct {
	repo     string // "owner/name"
	interval time.Duration
	client   *http.Client

	mu   sync.RWMutex
	code int
	url  string
}

// NewTracker returns nil when repo is empty, which callers treat as "release
// tracking disabled" and fall back to their static configuration.
func NewTracker(repo string, interval time.Duration) *Tracker {
	if repo == "" {
		return nil
	}
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Tracker{
		repo:     repo,
		interval: interval,
		client: &http.Client{
			Timeout: 10 * time.Second,
			// Keep the 302 instead of chasing it — the Location header is the
			// whole answer.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Latest reports the newest release seen so far. A zero code means "nothing
// known yet" — never a claim that no release exists.
func (t *Tracker) Latest() (int, string) {
	if t == nil {
		return 0, ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.code, t.url
}

// Run polls until ctx is cancelled. Safe to call on a nil Tracker.
func (t *Tracker) Run(ctx context.Context) {
	if t == nil {
		return
	}
	// Poll once up front so a server that has just started is not blind for a
	// full interval.
	t.refresh(ctx)

	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.refresh(ctx)
		}
	}
}

func (t *Tracker) refresh(ctx context.Context) {
	code, url, err := t.fetch(ctx)
	if err != nil {
		// A failed poll keeps the previous answer rather than blanking it —
		// a GitHub outage should not make every client think it is current.
		slog.Warn("release poll failed", "repo", t.repo, "err", err)
		return
	}
	t.mu.Lock()
	changed := code != t.code
	t.code, t.url = code, url
	t.mu.Unlock()
	if changed {
		slog.Info("latest release updated", "repo", t.repo, "versionCode", code, "url", url)
	}
}

func (t *Tracker) fetch(ctx context.Context) (int, string, error) {
	// Deliberately not the REST API: unauthenticated api.github.com allows 60
	// requests/hour *per IP*, and shared hosting egress shares that budget with
	// every other tenant, which returned 403 in practice. The plain releases
	// page instead 302s straight to the newest published release, needs no
	// token, and its redirect target is exactly the URL clients should open.
	endpoint := fmt.Sprintf("https://github.com/%s/releases/latest", t.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, endpoint, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "blindmap-server")

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	if loc == "" {
		// 200 here means the repo has no published release to redirect to.
		return 0, "", fmt.Errorf("no redirect from releases/latest (status %s)", resp.Status)
	}

	tag := loc[strings.LastIndex(loc, "/")+1:]
	code := VersionCodeFromTag(tag)
	if code == 0 {
		return 0, "", fmt.Errorf("tag %q is not vMAJOR.MINOR.PATCH", tag)
	}
	return code, loc, nil
}
