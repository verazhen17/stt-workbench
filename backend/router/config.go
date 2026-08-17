package router

import "os"

const (
	defaultHTTPAddr     = ":8080"
	defaultVODRoot      = "/app/data/vod"
	defaultSTTRoot      = "/app/data/stt"
	defaultGoldenRoot   = "/app/data/golden"
	defaultVODURLPrefix = "/vod"
	defaultFFprobePath  = "/usr/bin/ffprobe"
)

type Config struct {
	HTTPAddr     string
	VODRoot      string
	STTRoot      string
	GoldenRoot   string
	VODURLPrefix string
	FFprobePath  string
}

func LoadConfig() Config {
	return Config{
		HTTPAddr:     valueOrDefault("HTTP_ADDR", defaultHTTPAddr),
		VODRoot:      valueOrDefault("VOD_ROOT", defaultVODRoot),
		STTRoot:      valueOrDefault("STT_ROOT", defaultSTTRoot),
		GoldenRoot:   valueOrDefault("GOLDEN_ROOT", defaultGoldenRoot),
		VODURLPrefix: valueOrDefault("VOD_URL_PREFIX", defaultVODURLPrefix),
		FFprobePath:  valueOrDefault("FFPROBE_PATH", defaultFFprobePath),
	}
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
