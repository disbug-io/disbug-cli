package logparse

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const maxLogEntriesPerCategory = 10000

const (
	reasonTruncated        = "Body was cut off before upload (capture size limit)."
	reasonBinary           = "Body is binary and was not stored as text."
	reasonEmptyRequest     = "No body was stored for this request."
	reasonEmptyResponse    = "No body was stored for this response."
	reasonFullRequest      = "Request body captured in full."
	reasonFullResponse     = "Response body captured in full."
	reasonResourceBody     = "Bodies are not captured for static resource timing entries; use fetch/XHR rows for bodies."
	reasonEntryTruncated   = "Entire log entry was truncated before upload; details are unavailable."
	reasonRedacted         = "Sensitive header or body values are hidden in this report."
	captureFullBoth        = "Request and response bodies stored in full."
	captureFullRequest     = "Request body stored in full."
	captureFullResponse    = "Response body stored in full."
	redactedMarker         = "[redacted]"
	curlBodyOmittedComment = "# Request body not stored in full; body omitted from replay."
)

var sensitiveHeaderNames = map[string]struct{}{
	"authorization":       {},
	"cookie":              {},
	"set-cookie":          {},
	"proxy-authorization": {},
	"x-api-key":           {},
	"x-auth-token":        {},
}

// ParseNetworkLogs normalizes extension network_logs to the same shape as apps/sessions/logs.py.
func ParseNetworkLogs(items []map[string]any) []map[string]any {
	if len(items) > maxLogEntriesPerCategory {
		items = items[:maxLogEntriesPerCategory]
	}
	out := make([]map[string]any, 0, len(items))
	for index, item := range items {
		if item == nil {
			continue
		}
		normalized := normalizeNetworkItem(item)
		normalized["id"] = strconv.Itoa(index + 1)
		out = append(out, normalized)
	}
	return out
}

func normalizeNetworkItem(item map[string]any) map[string]any {
	if isTruncatedEntry(item) {
		preview := truncatedPreview(item)
		row := map[string]any{
			"kind":              "truncated",
			"display_type":      "other",
			"resource_type":     "other",
			"method":            "-",
			"url":               "Entry truncated",
			"status":            nil,
			"status_label":      "Truncated",
			"status_family":     "unknown",
			"duration_ms":       nil,
			"duration_label":    "-",
			"request_headers":   []map[string]string{},
			"response_headers":  []map[string]string{},
			"request_body":      "",
			"response_body":     "",
			"error_detail":      "",
			"is_truncated":      true,
			"truncated_preview": preview,
			"raw_json":          prettyValue(item),
		}
		for key, value := range agentFieldsForTruncatedEntry() {
			row[key] = value
		}
		return row
	}

	rawRequest := asMap(item["request"])
	rawResponse := asMap(item["response"])
	responseExtra := asMap(rawResponse["extra"])

	method := stringValue(item["method"], rawRequest["verb"], rawRequest["method"])
	url := stringValue(item["url"], rawRequest["uri"], rawRequest["url"], responseExtra["url"])

	status := item["status"]
	if status == nil {
		status = rawResponse["status"]
	}

	kind := strings.ToLower(stringValue(item["type"]))
	if kind == "" {
		kind = "request"
	}
	resourceType := networkResourceType(kind, url)

	requestHeaders := headerEntries(rawRequest["headers"])
	responseHeaders := headerEntries(rawResponse["headers"])
	requestBody := prettyValue(rawRequest["body"])
	responseBody := prettyValue(rawResponse["body"])

	meta := networkCaptureMeta(requestHeaders, responseHeaders, requestBody, responseBody, rawRequest, rawResponse, kind)
	curlSafe := buildCurlSafe(stringValue(method), url, requestHeaders, requestBody, meta.RequestBodyState, false)

	row := map[string]any{
		"kind":                       kind,
		"display_type":               resourceType,
		"resource_type":              resourceType,
		"method":                     firstNonEmpty(stringValue(method), "-"),
		"url":                        firstNonEmpty(url, "-"),
		"status":                     status,
		"status_label":               statusLabel(status),
		"status_family":              statusFamily(status),
		"duration_label":             "-",
		"request_headers":            requestHeaders,
		"response_headers":           responseHeaders,
		"request_body":               requestBody,
		"response_body":              responseBody,
		"error_detail":               prettyValue(firstNonNil(rawResponse["error"], item["error"])),
		"is_truncated":               false,
		"truncated_preview":          "",
		"raw_json":                   prettyValue(item),
		"curl_safe":                  curlSafe,
		"request_body_state":         meta.RequestBodyState,
		"response_body_state":        meta.ResponseBodyState,
		"request_body_state_reason":  meta.RequestBodyStateReason,
		"response_body_state_reason": meta.ResponseBodyStateReason,
		"capture_status":             meta.CaptureStatus,
		"has_redacted_headers":       meta.HasRedactedHeaders,
		"has_redacted_body":          meta.HasRedactedBody,
		"row_flag_partial":           meta.RowFlagPartial,
		"row_flag_redacted":          meta.RowFlagRedacted,
	}
	return row
}

func isTruncatedEntry(item map[string]any) bool {
	truncated, ok := item["_disbugTruncated"].(bool)
	return ok && truncated
}

func truncatedPreview(item map[string]any) string {
	preview := item["preview"]
	if preview == nil || fmt.Sprint(preview) == "" {
		return "Preview unavailable."
	}
	return fmt.Sprint(preview)
}

func asMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func stringValue(values ...any) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if typed != "" {
				return typed
			}
		default:
			text := fmt.Sprint(typed)
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func networkResourceType(kind, url string) string {
	normalizedKind := strings.ToLower(kind)
	normalizedURL := strings.ToLower(url)
	switch normalizedKind {
	case "fetch", "xhr":
		return normalizedKind
	case "link":
		if strings.Contains(normalizedURL, ".js") {
			return "script"
		}
		return "css"
	case "css", "stylesheet", "style":
		return "css"
	case "script", "js", "javascript":
		return "script"
	case "img", "image", "images":
		return "img"
	default:
		return "other"
	}
}

func statusFamily(status any) string {
	code, err := strconv.Atoi(fmt.Sprint(status))
	if err != nil {
		return "unknown"
	}
	switch {
	case code == 0:
		return "failed"
	case code >= 200 && code < 300:
		return "success"
	case code >= 300 && code < 400:
		return "redirect"
	case code >= 400 && code < 500:
		return "client-error"
	case code >= 500:
		return "server-error"
	default:
		return "unknown"
	}
}

func statusLabel(status any) string {
	if statusFamily(status) == "failed" {
		return "Failed"
	}
	if status == nil {
		return "-"
	}
	return fmt.Sprint(status)
}

func headerEntries(headers any) []map[string]string {
	switch typed := headers.(type) {
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			return strings.ToLower(names[i]) < strings.ToLower(names[j])
		})
		entries := make([]map[string]string, 0, len(names))
		for _, name := range names {
			entries = append(entries, map[string]string{
				"name":  name,
				"value": compactValue(typed[name], 1000),
			})
		}
		return entries
	case string:
		lines := strings.Split(typed, "\n")
		entries := make([]map[string]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				entries = append(entries, map[string]string{"name": line, "value": ""})
				continue
			}
			entries = append(entries, map[string]string{"name": strings.TrimSpace(name), "value": strings.TrimSpace(value)})
		}
		return entries
	case []any:
		entries := make([]map[string]string, 0, len(typed))
		for _, entry := range typed {
			switch row := entry.(type) {
			case map[string]any:
				name := stringValue(row["name"], row["key"])
				if name == "" {
					continue
				}
				entries = append(entries, map[string]string{
					"name":  name,
					"value": compactValue(row["value"], 1000),
				})
			}
		}
		return entries
	default:
		return []map[string]string{}
	}
}

func prettyValue(value any) string {
	if value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return typed
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			encoded, err := json.MarshalIndent(parsed, "", "  ")
			if err == nil {
				return string(encoded)
			}
		}
		return typed
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

func compactValue(value any, maxLength int) string {
	text := prettyValue(value)
	if len(text) <= maxLength {
		return text
	}
	return text[:maxLength] + "... [truncated]"
}

func bodyCaptureState(body string) string {
	if body == "" {
		return "empty"
	}
	lowered := strings.ToLower(body)
	if strings.Contains(lowered, "[binary") {
		return "binary"
	}
	if strings.Contains(lowered, "[truncated") || strings.Contains(lowered, "too large") {
		return "truncated"
	}
	return "full"
}

func resolveBodyState(side map[string]any, body string, isFetchXHR bool) string {
	if side != nil {
		if explicit, ok := side["body_state"].(string); ok && strings.TrimSpace(explicit) != "" {
			return strings.ToLower(strings.TrimSpace(explicit))
		}
	}
	if !isFetchXHR && body == "" {
		return "empty"
	}
	return bodyCaptureState(body)
}

func bodyStateReason(state string, side map[string]any, isRequest bool, isFetchXHR bool, hasBodyText bool) string {
	if side != nil {
		if explicit, ok := side["body_state_reason"].(string); ok && strings.TrimSpace(explicit) != "" {
			return strings.TrimSpace(explicit)
		}
	}
	if !isFetchXHR {
		if state == "empty" && !hasBodyText {
			return reasonResourceBody
		}
	}
	return reasonForState(state, isRequest)
}

func reasonForState(state string, isRequest bool) string {
	switch state {
	case "truncated":
		return reasonTruncated
	case "binary":
		return reasonBinary
	case "full":
		if isRequest {
			return reasonFullRequest
		}
		return reasonFullResponse
	case "empty":
		if isRequest {
			return reasonEmptyRequest
		}
		return reasonEmptyResponse
	default:
		return ""
	}
}

func headersHaveRedaction(headers []map[string]string) bool {
	for _, header := range headers {
		if strings.Contains(strings.ToLower(header["value"]), redactedMarker) {
			return true
		}
	}
	return false
}

func bodyHasRedaction(body string) bool {
	return strings.Contains(strings.ToLower(body), redactedMarker)
}

type captureMeta struct {
	RequestBodyState        string
	ResponseBodyState       string
	RequestBodyStateReason  string
	ResponseBodyStateReason string
	CaptureStatus           []map[string]string
	HasRedactedHeaders      bool
	HasRedactedBody         bool
	RowFlagPartial          bool
	RowFlagRedacted         bool
}

func networkCaptureMeta(
	requestHeaders, responseHeaders []map[string]string,
	requestBody, responseBody string,
	rawRequest, rawResponse map[string]any,
	kind string,
) captureMeta {
	isFetchXHR := kind == "fetch" || kind == "xhr"
	requestState := resolveBodyState(rawRequest, requestBody, isFetchXHR)
	responseState := resolveBodyState(rawResponse, responseBody, isFetchXHR)
	hasRedactedHeaders := headersHaveRedaction(requestHeaders) || headersHaveRedaction(responseHeaders)
	hasRedactedBody := bodyHasRedaction(requestBody) || bodyHasRedaction(responseBody)

	requestReason := bodyStateReason(requestState, rawRequest, true, isFetchXHR, requestBody != "")
	responseReason := bodyStateReason(responseState, rawResponse, false, isFetchXHR, responseBody != "")

	return captureMeta{
		RequestBodyState:        requestState,
		ResponseBodyState:       responseState,
		RequestBodyStateReason:  requestReason,
		ResponseBodyStateReason: responseReason,
		CaptureStatus:           buildCaptureStatus(requestState, responseState, hasRedactedHeaders, hasRedactedBody, requestReason, responseReason),
		HasRedactedHeaders:      hasRedactedHeaders,
		HasRedactedBody:         hasRedactedBody,
		RowFlagPartial: requestState == "truncated" || requestState == "binary" ||
			responseState == "truncated" || responseState == "binary",
		RowFlagRedacted: hasRedactedHeaders || hasRedactedBody,
	}
}

func buildCaptureStatus(
	requestState, responseState string,
	hasRedactedHeaders, hasRedactedBody bool,
	requestReason, responseReason string,
) []map[string]string {
	rows := []map[string]string{}
	if requestState == "full" && responseState == "full" {
		rows = append(rows, map[string]string{"tone": "ok", "label": "Full", "detail": captureFullBoth})
	} else {
		if requestState == "full" {
			rows = append(rows, map[string]string{"tone": "ok", "label": "Full", "detail": captureFullRequest})
		} else if requestState == "truncated" {
			rows = append(rows, map[string]string{"tone": "warn", "label": "Truncated", "detail": firstNonEmpty(requestReason, reasonTruncated)})
		} else if requestState == "binary" {
			rows = append(rows, map[string]string{"tone": "warn", "label": "Binary", "detail": firstNonEmpty(requestReason, reasonBinary)})
		}
		if responseState == "truncated" {
			rows = append(rows, map[string]string{"tone": "warn", "label": "Truncated", "detail": firstNonEmpty(responseReason, reasonTruncated)})
		} else if responseState == "binary" {
			rows = append(rows, map[string]string{"tone": "warn", "label": "Binary", "detail": firstNonEmpty(responseReason, reasonBinary)})
		} else if responseState == "full" {
			rows = append(rows, map[string]string{"tone": "ok", "label": "Full", "detail": captureFullResponse})
		}
	}
	if hasRedactedHeaders || hasRedactedBody {
		rows = append(rows, map[string]string{"tone": "neutral", "label": "Redacted", "detail": reasonRedacted})
	}
	return rows
}

func agentFieldsForTruncatedEntry() map[string]any {
	return map[string]any{
		"request_body_state":         "empty",
		"response_body_state":        "empty",
		"request_body_state_reason":  reasonEntryTruncated,
		"response_body_state_reason": reasonEntryTruncated,
		"capture_status": []map[string]string{
			{"tone": "warn", "label": "Truncated", "detail": reasonEntryTruncated},
		},
		"has_redacted_headers": false,
		"has_redacted_body":    false,
		"row_flag_partial":     true,
		"row_flag_redacted":    false,
		"curl_safe":            "",
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isRedactedValue(value string) bool {
	return strings.Contains(strings.ToLower(value), redactedMarker)
}

func buildCurlSafe(
	method, url string,
	requestHeaders []map[string]string,
	requestBody, requestBodyState string,
	isTruncatedEntry bool,
) string {
	if isTruncatedEntry || url == "" || url == "-" {
		return ""
	}
	normalizedMethod := strings.ToUpper(firstNonEmpty(method, "GET"))
	parts := []string{fmt.Sprintf("curl -X %s %s", normalizedMethod, shellQuote(url))}
	for _, header := range requestHeaders {
		name := header["name"]
		lowered := strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, sensitive := sensitiveHeaderNames[lowered]; sensitive {
			continue
		}
		if isRedactedValue(header["value"]) {
			continue
		}
		parts = append(parts, fmt.Sprintf("  -H %s", shellQuote(fmt.Sprintf("%s: %s", name, header["value"]))))
	}
	bodyState := strings.ToLower(firstNonEmpty(requestBodyState, bodyCaptureState(requestBody)))
	if bodyState == "truncated" || bodyState == "binary" {
		parts = append(parts, "  "+curlBodyOmittedComment)
	} else if requestBody != "" && normalizedMethod != "GET" && normalizedMethod != "HEAD" {
		parts = append(parts, fmt.Sprintf("  --data-raw %s", shellQuote(requestBody)))
	}
	return strings.Join(parts, " \\\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
