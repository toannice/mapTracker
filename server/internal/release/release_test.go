package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVersionCodeFromTag(t *testing.T) {
	cases := []struct {
		tag  string
		want int
	}{
		{"v1.0.3", 10003},
		{"1.0.3", 10003}, // tolerate a missing v
		{"v1.2.3", 10203},
		{"v0.0.1", 1},
		{"v12.34.56", 123456},
		{"v1.0", 0},       // not three parts
		{"v1.0.3-rc1", 0}, // suffix is not a number
		{"nightly", 0},    // not a version at all
		{"v1.100.0", 0},   // minor would collide with the next major
		{"v1.0.100", 0},   // patch would collide with the next minor
		{"v-1.0.0", 0},    // negative
	}
	for _, c := range cases {
		if got := VersionCodeFromTag(c.tag); got != c.want {
			t.Errorf("VersionCodeFromTag(%q) = %d, want %d", c.tag, got, c.want)
		}
	}
}

// The formula must stay in lockstep with .github/workflows/release.yml, which
// computes MAJOR*10000 + MINOR*100 + PATCH.
func TestVersionCodeMatchesWorkflowFormula(t *testing.T) {
	for _, c := range []struct{ maj, min, patch int }{{1, 0, 3}, {2, 15, 7}, {0, 9, 99}} {
		want := c.maj*10000 + c.min*100 + c.patch
		tag := "v" + itoa(c.maj) + "." + itoa(c.min) + "." + itoa(c.patch)
		if got := VersionCodeFromTag(tag); got != want {
			t.Errorf("%s = %d, want %d", tag, got, want)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestNilTrackerIsInert(t *testing.T) {
	var tr *Tracker = NewTracker("", time.Minute)
	if tr != nil {
		t.Fatal("empty repo should disable tracking")
	}
	code, url := tr.Latest()
	if code != 0 || url != "" {
		t.Errorf("nil tracker returned %d %q", code, url)
	}
	tr.Run(context.Background()) // must not panic or block
}

func redirectServer(t *testing.T, location string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if location != "" {
			w.Header().Set("Location", location)
		}
		w.WriteHeader(status)
	}))
}

func TestRefreshReadsTagFromRedirect(t *testing.T) {
	srv := redirectServer(t, "https://github.com/o/n/releases/tag/v2.1.4", http.StatusFound)
	defer srv.Close()

	tr := newTestTracker(srv.URL)
	tr.refresh(context.Background())

	code, url := tr.Latest()
	if code != 20104 {
		t.Errorf("code = %d, want 20104", code)
	}
	if url != "https://github.com/o/n/releases/tag/v2.1.4" {
		t.Errorf("url = %q, want the redirect target", url)
	}
}

// A repo with no published release answers 200 with no Location.
func TestNoReleaseYieldsNothing(t *testing.T) {
	srv := redirectServer(t, "", http.StatusOK)
	defer srv.Close()

	tr := newTestTracker(srv.URL)
	tr.refresh(context.Background())
	if code, _ := tr.Latest(); code != 0 {
		t.Errorf("code = %d, want 0 when there is no release", code)
	}
}

// A failed poll must keep the last good answer rather than blanking it — a
// GitHub outage should not silently tell every client it is up to date.
func TestFailedRefreshKeepsPreviousAnswer(t *testing.T) {
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Location", "https://github.com/o/n/releases/tag/v1.0.3")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	tr := newTestTracker(srv.URL)
	tr.refresh(context.Background())
	fail = true
	tr.refresh(context.Background())

	if code, _ := tr.Latest(); code != 10003 {
		t.Errorf("code = %d after failed poll, want 10003 retained", code)
	}
}

// The redirect must not be followed, or Location is lost and the tag with it.
func TestRedirectIsNotFollowed(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Location", "https://github.com/o/n/releases/tag/v1.0.3")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	tr := newTestTracker(srv.URL)
	tr.refresh(context.Background())
	if hits != 1 {
		t.Errorf("server hit %d times, want 1 — the redirect was chased", hits)
	}
}

// Points fetch at a local test server by overriding the endpoint host.
func newTestTracker(base string) *Tracker {
	tr := NewTracker("owner/name", time.Minute)
	tr.client = &http.Client{
		Timeout:   5 * time.Second,
		Transport: rewriteHost{base: base},
		// Must mirror NewTracker: following the redirect discards Location,
		// which is the only thing fetch actually reads.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return tr
}

type rewriteHost struct{ base string }

func (r rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	orig, err := http.NewRequest(req.Method, r.base, nil)
	if err != nil {
		return nil, err
	}
	u.Scheme, u.Host = orig.URL.Scheme, orig.URL.Host
	req = req.Clone(req.Context())
	req.URL = &u
	return http.DefaultTransport.RoundTrip(req)
}
