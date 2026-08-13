package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port           string
	LogLevel       string
	MaxRooms       int
	TurnSeconds    int
	MapSize        int
	AllowedOrigins []string

	// Client auto-update hints, echoed in every welcome message. Zero / empty
	// means "no opinion" and clients show nothing, so an unconfigured server
	// behaves exactly as it did before.
	LatestVersionCode int
	MinVersionCode    int
	UpdateURL         string

	// Releases, when set, reports the newest published build by polling
	// GitHub. It wins over LatestVersionCode/UpdateURL above, which remain the
	// manual override for deployments that do not track a repo.
	Releases ReleaseSource

	// GitHubRepo ("owner/name") enables release tracking; empty disables it.
	GitHubRepo         string
	ReleasePollMinutes int
}

// ReleaseSource is the slice of release tracking that Config needs. Kept as an
// interface so config does not depend on the polling implementation.
type ReleaseSource interface {
	// Latest returns the newest known Android versionCode and its download
	// page. A zero code means "nothing known yet", not "no release exists".
	Latest() (int, string)
}

// UpdateHints resolves what to tell clients about available updates. Live
// release data wins when present; otherwise the static settings apply. Callers
// send the result as-is — zeroes mean "say nothing", which is what an
// unconfigured server should do.
func (c *Config) UpdateHints() (latest, minimum int, url string) {
	latest, url = c.LatestVersionCode, c.UpdateURL
	if c.Releases != nil {
		if code, u := c.Releases.Latest(); code > 0 {
			latest, url = code, u
		}
	}
	return latest, c.MinVersionCode, url
}

func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		MaxRooms:       getEnvInt("MAX_ROOMS", 100),
		TurnSeconds:    getEnvInt("TURN_SECONDS", 30),
		MapSize:        getEnvInt("MAP_SIZE", 20),
		AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "*"), ","),

		LatestVersionCode:  getEnvInt("LATEST_VERSION_CODE", 0),
		MinVersionCode:     getEnvInt("MIN_VERSION_CODE", 0),
		UpdateURL:          getEnv("UPDATE_URL", ""),
		GitHubRepo:         getEnv("GITHUB_REPO", ""),
		ReleasePollMinutes: getEnvInt("RELEASE_POLL_MINUTES", 10),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
