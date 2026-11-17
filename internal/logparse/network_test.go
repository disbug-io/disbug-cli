package logparse

import (
	"strings"
	"testing"
)

func TestParseNetworkLogsIncludesAgentParityFields(t *testing.T) {
	rows := ParseNetworkLogs([]map[string]any{
		{
			"type": "fetch",
			"request": map[string]any{
				"verb":    "POST",
				"uri":     "https://api.example.test/save",
				"headers": map[string]any{"content-type": "application/json"},
				"body":    `{"ok":true}`,
			},
			"response": map[string]any{
				"status": 200,
				"body":   "{}",
			},
		},
	})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row["id"] != "1" {
		t.Fatalf("id = %v, want 1", row["id"])
	}
	if _, ok := row["curl_safe"]; !ok {
		t.Fatal("curl_safe missing")
	}
	if curl, _ := row["curl_safe"].(string); curl == "" || !strings.Contains(curl, "curl -X POST") {
		t.Fatalf("curl_safe = %q, want POST curl", curl)
	}
	if _, ok := row["capture_status"]; !ok {
		t.Fatal("capture_status missing")
	}
	if reason, _ := row["request_body_state_reason"].(string); reason == "" {
		t.Fatal("request_body_state_reason missing")
	}
}

func TestBuildCurlSafeOmitsTruncatedRequestBody(t *testing.T) {
	curl := buildCurlSafe(
		"POST",
		"https://api.example.test/save",
		[]map[string]string{{"name": "Content-Type", "value": "application/json"}},
		"[truncated - too large]",
		"truncated",
		false,
	)
	if strings.Contains(curl, "--data-raw") {
		t.Fatalf("curl_safe = %q, want no --data-raw", curl)
	}
	if !strings.Contains(strings.ToLower(curl), "omitted") {
		t.Fatalf("curl_safe = %q, want omitted comment", curl)
	}
}

func TestParseNetworkLogsLegacyRowStillNormalizes(t *testing.T) {
	rows := ParseNetworkLogs([]map[string]any{
		{"url": "/api/items", "status": 500},
	})
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0]["url"] != "/api/items" {
		t.Fatalf("url = %v, want /api/items", rows[0]["url"])
	}
}
