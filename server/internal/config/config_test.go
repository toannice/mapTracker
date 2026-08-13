package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("MAX_ROOMS")
	os.Unsetenv("TURN_SECONDS")
	os.Unsetenv("MAP_SIZE")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Port: want 8080, got %s", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: want info, got %s", cfg.LogLevel)
	}
	if cfg.MaxRooms != 100 {
		t.Errorf("MaxRooms: want 100, got %d", cfg.MaxRooms)
	}
	if cfg.TurnSeconds != 30 {
		t.Errorf("TurnSeconds: want 30, got %d", cfg.TurnSeconds)
	}
	if cfg.MapSize != 20 {
		t.Errorf("MapSize: want 20, got %d", cfg.MapSize)
	}
}

func TestLoadOverrides(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("MAX_ROOMS", "50")
	os.Setenv("TURN_SECONDS", "10")
	os.Setenv("MAP_SIZE", "5")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("MAX_ROOMS")
		os.Unsetenv("TURN_SECONDS")
		os.Unsetenv("MAP_SIZE")
	}()

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Port: want 9090, got %s", cfg.Port)
	}
	if cfg.MaxRooms != 50 {
		t.Errorf("MaxRooms: want 50, got %d", cfg.MaxRooms)
	}
	if cfg.MapSize != 5 {
		t.Errorf("MapSize: want 5, got %d", cfg.MapSize)
	}
}

type stubReleases struct {
	code int
	url  string
}

func (s stubReleases) Latest() (int, string) { return s.code, s.url }

func TestUpdateHintsPrefersLiveReleaseData(t *testing.T) {
	cfg := Config{
		LatestVersionCode: 10001,
		MinVersionCode:    9000,
		UpdateURL:         "https://static/url",
		Releases:          stubReleases{code: 10003, url: "https://live/url"},
	}
	latest, minimum, url := cfg.UpdateHints()
	if latest != 10003 || url != "https://live/url" {
		t.Errorf("got %d %q, want live values", latest, url)
	}
	if minimum != 9000 {
		t.Errorf("minimum = %d, want the static 9000 (never tracked)", minimum)
	}
}

// Until the first poll lands the tracker reports 0, which must not wipe out a
// manually configured version.
func TestUpdateHintsFallsBackBeforeFirstPoll(t *testing.T) {
	cfg := Config{
		LatestVersionCode: 10001,
		UpdateURL:         "https://static/url",
		Releases:          stubReleases{code: 0},
	}
	latest, _, url := cfg.UpdateHints()
	if latest != 10001 || url != "https://static/url" {
		t.Errorf("got %d %q, want the static fallback", latest, url)
	}
}

func TestUpdateHintsSilentWhenUnconfigured(t *testing.T) {
	latest, minimum, url := (&Config{}).UpdateHints()
	if latest != 0 || minimum != 0 || url != "" {
		t.Errorf("unconfigured server advertised %d %d %q", latest, minimum, url)
	}
}
