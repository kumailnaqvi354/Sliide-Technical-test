package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestNewLoggerLevels(t *testing.T) {
	tests := []struct {
		level       string
		wantDebug   bool
		wantInfo    bool
		wantUnknown bool
	}{
		{level: "", wantInfo: true},
		{level: "debug", wantDebug: true, wantInfo: true},
		{level: "DEBUG", wantDebug: true, wantInfo: true},
		{level: "info", wantInfo: true},
		{level: "warn"},
		{level: "error"},
		{level: "loud", wantInfo: true, wantUnknown: true},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var out bytes.Buffer
			logger, unknown := newLogger(&out, tt.level)

			if unknown != tt.wantUnknown {
				t.Errorf("unknown = %v, want %v", unknown, tt.wantUnknown)
			}

			logger.Debug("d")
			if got := out.Len() > 0; got != tt.wantDebug {
				t.Errorf("debug logged = %v, want %v", got, tt.wantDebug)
			}

			out.Reset()
			logger.Info("i")
			if got := out.Len() > 0; got != tt.wantInfo {
				t.Errorf("info logged = %v, want %v", got, tt.wantInfo)
			}
		})
	}
}

func TestNewLoggerWritesJSONWithServiceName(t *testing.T) {
	var out bytes.Buffer
	logger, _ := newLogger(&out, "")

	logger.Info("hello", "article_id", "abc")

	var line map[string]any
	if err := json.Unmarshal(out.Bytes(), &line); err != nil {
		t.Fatalf("not JSON: %v: %s", err, out.String())
	}

	for key, want := range map[string]string{
		"level": "INFO", "msg": "hello", "service": serviceName, "article_id": "abc",
	} {
		if line[key] != want {
			t.Errorf("%s = %v, want %q", key, line[key], want)
		}
	}
}
