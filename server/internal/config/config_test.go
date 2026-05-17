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
