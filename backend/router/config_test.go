package router_test

import (
	"testing"

	"github.com/17media/stt-workbench/backend/router"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{
		"HTTP_ADDR",
		"VOD_ROOT",
		"STT_ROOT",
		"GOLDEN_ROOT",
		"VOD_URL_PREFIX",
		"FFPROBE_PATH",
	} {
		t.Setenv(key, "")
	}

	got := router.LoadConfig()
	want := router.Config{
		HTTPAddr:     ":8080",
		VODRoot:      "/app/data/vod",
		STTRoot:      "/app/data/stt",
		GoldenRoot:   "/app/data/golden",
		VODURLPrefix: "/vod",
		FFprobePath:  "/usr/bin/ffprobe",
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	t.Setenv("VOD_ROOT", "/tmp/vod")
	t.Setenv("STT_ROOT", "/tmp/stt")
	t.Setenv("GOLDEN_ROOT", "/tmp/golden")
	t.Setenv("VOD_URL_PREFIX", "/media")
	t.Setenv("FFPROBE_PATH", "/opt/bin/ffprobe")

	got := router.LoadConfig()
	want := router.Config{
		HTTPAddr:     "127.0.0.1:9090",
		VODRoot:      "/tmp/vod",
		STTRoot:      "/tmp/stt",
		GoldenRoot:   "/tmp/golden",
		VODURLPrefix: "/media",
		FFprobePath:  "/opt/bin/ffprobe",
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}
