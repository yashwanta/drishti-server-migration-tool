package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func capture(level string) (*Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return New(&buf, level), &buf
}

func TestSecretsAreRedacted(t *testing.T) {
	cases := []struct {
		key string
		val any
	}{
		{"password", "hunter2"},
		{"api_key", "abc123"},
		{"token", "pve-token-xyz"},
		{"Authorization", "Bearer secretjwt"},
		{"db_url", "postgres://u:supersecret@host/db"},
		{"private_key", "-----BEGIN PRIVATE KEY-----"},
	}
	for _, c := range cases {
		log, buf := capture("info")
		log.Info("test", Field{Key: c.key, Value: c.val})
		out := buf.String()
		var rec map[string]any
		if err := json.Unmarshal([]byte(out), &rec); err != nil {
			t.Fatalf("invalid json for %s: %v\n%s", c.key, err, out)
		}
		got, _ := rec[c.key].(string)
		if strings.Contains(strings.ToLower(got), "secret") || strings.Contains(got, "hunter2") || strings.Contains(got, "supersecret") {
			t.Errorf("secret leaked for key %q: %q", c.key, got)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	log, buf := capture("warn")
	log.Info("should not appear")
	log.Warn("should appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("info leaked above warn level")
	}
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("warn message missing")
	}
}

func TestStructuredFields(t *testing.T) {
	log, buf := capture("info")
	log.With(Field{Key: "component", Value: "api"}).Info("handled", Field{Key: "status", Value: 200})
	var rec map[string]any
	_ = json.Unmarshal(buf.Bytes(), &rec)
	if rec["component"] != "api" || rec["status"] != float64(200) {
		t.Errorf("fields not merged: %v", rec)
	}
}

func TestSafeStringValue(t *testing.T) {
	log, buf := capture("info")
	log.Info("ok", Field{Key: "endpoint", Value: "https://vcenter.lab.local/sdk"})
	var rec map[string]any
	_ = json.Unmarshal(buf.Bytes(), &rec)
	if rec["endpoint"] != "https://vcenter.lab.local/sdk" {
		t.Errorf("safe value altered: %v", rec["endpoint"])
	}
}