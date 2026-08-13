package config

import (
	"testing"
	"time"
)

func TestLoadParsesCacheTTL(t *testing.T) {
	t.Setenv("CACHE_TTL", "45s")
	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settings.CacheTTL != 45*time.Second {
		t.Fatalf("CacheTTL = %s, want 45s", settings.CacheTTL)
	}
}

func TestLoadRejectsInvalidCacheTTL(t *testing.T) {
	t.Setenv("CACHE_TTL", "amanhã")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid CACHE_TTL error")
	}
}

func TestLoadCanDisableCacheForBaseline(t *testing.T) {
	t.Setenv("CACHE_ENABLED", "false")
	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settings.CacheEnabled {
		t.Fatal("CacheEnabled = true, want false")
	}
}

func TestLoadRejectsInvalidCacheEnabled(t *testing.T) {
	t.Setenv("CACHE_ENABLED", "talvez")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid CACHE_ENABLED error")
	}
}
