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
}

func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8080"),
		LogLevel:       getEnv("LOG_LEVEL", "info"),
		MaxRooms:       getEnvInt("MAX_ROOMS", 100),
		TurnSeconds:    getEnvInt("TURN_SECONDS", 30),
		MapSize:        getEnvInt("MAP_SIZE", 20),
		AllowedOrigins: strings.Split(getEnv("ALLOWED_ORIGINS", "*"), ","),

		LatestVersionCode: getEnvInt("LATEST_VERSION_CODE", 0),
		MinVersionCode:    getEnvInt("MIN_VERSION_CODE", 0),
		UpdateURL:         getEnv("UPDATE_URL", ""),
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
