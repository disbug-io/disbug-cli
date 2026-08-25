package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const e2eTimeout = 5 * time.Second

var e2eTools = []string{
	"whoami",
	"list_sessions",
	"get_session",
	"get_pin",
	"get_pins",
	"download_attachment",
	"inspect_local_report",
	"search_sessions",
	"search_pins",
	"set_session_status",
	"set_pin_status",
}

func TestMCPSubprocessInitialize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess MCP E2E test in short mode")
	}

	binary := buildBinary(t)
	proc := startMCP(t, binary, t.TempDir())
	defer proc.close(t)

	resp := initializeMCP(t, proc)
	if !strings.Contains(string(resp.Result), `"serverInfo"`) {
		t.Fatalf("initialize result = %s, want serverInfo", resp.Result)
	}
	if !strings.Contains(string(resp.Result), `"protocolVersion"`) {
		t.Fatalf("initialize result = %s, want protocolVersion", resp.Result)
	}
	if !strings.Contains(string(resp.Result), `"instructions"`) || !strings.Contains(string(resp.Result), "smallest evidence fields") {
		t.Fatalf("initialize result = %s, want workflow instructions", resp.Result)
	}
}

func TestMCPSubprocessToolsListIncludesAllTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess MCP E2E test in short mode")
	}

	binary := buildBinary(t)
	proc := startMCP(t, binary, t.TempDir())
	defer proc.close(t)

	initializeMCP(t, proc)
	resp := rpcCall(t, proc, 2, "tools/list", nil)

	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/list result: %v; raw = %s", err, resp.Result)
	}

	got := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	for _, want := range e2eTools {
		if !got[want] {
			t.Fatalf("tools/list names = %v, want %q", got, want)
		}
	}
}

func TestMCPSubprocessEOFShutdownExitsPromptly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess MCP E2E test in short mode")
	}

	binary := buildBinary(t)
	proc := startMCP(t, binary, t.TempDir())

	if err := proc.stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	waitForExit(t, proc, e2eTimeout)
}

func TestMCPSubprocessInspectLocalReportWithoutCloud(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess MCP E2E test in short mode")
	}

	binary := buildBinary(t)
	reportPath := writeMCPLocalReport(t)
	proc := startMCP(t, binary, t.TempDir())
	defer proc.close(t)

	initializeMCP(t, proc)
	text := callToolE2E(t, proc, "inspect_local_report", map[string]any{
		"path":   reportPath,
		"pin":    1,
		"fields": []string{"console"},
	})

	for _, want := range []string{`"source":"local"`, `"number":1`, "button missing", "boom"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspect_local_report content = %q, want %q", text, want)
		}
	}
}

func TestMCPSubprocessToolsCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess MCP E2E test in short mode")
	}

	binary := buildBinary(t)

	tests := []struct {
		name       string
		tool       string
		arguments  map[string]any
		handler    http.HandlerFunc
		wantText   []string
		wantPaths  []string
		wantQuery  map[string]string
		pathPrefix string
	}{
		{
			name:      "whoami",
			tool:      "whoami",
			arguments: map[string]any{},
			handler:   e2eBackendHandler(t, nil),
			wantText:  []string{`"agent_name":"a"`, `"team":"T"`, `"team_slug":"t"`},
		},
		{
			name:      "list_sessions",
			tool:      "list_sessions",
			arguments: map[string]any{"status": "open", "project": "web", "limit": 2},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/sessions/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"results":[{
							"id":7392,
							"team_slug":"abb",
							"project":{"id":2,"slug":"2","name":"Web"},
							"project_session_number":5,
							"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
							"url":"https://example.test/checkout",
							"status":"open",
							"pin_count":2,
							"first_pin_feedback":"checkout button broken",
							"reporter":{"email":"r@example.test","display_name":"Reporter"},
							"updated_at":"2026-05-01T00:00:00Z",
							"free_tier_locked":false
						}],
						"next_cursor":null,
						"count":1,
						"free_tier_truncated":false
					}`)
				},
			}),
			wantText:  []string{`"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/"`, `"status":"open"`, "checkout button broken"},
			wantPaths: []string{"/api/sessions/"},
			wantQuery: map[string]string{"status": "open", "project": "web", "limit": "2"},
		},
		{
			name:      "get_session",
			tool:      "get_session",
			arguments: map[string]any{"target": "https://staging.disbug.us/abb/projects/2/sessions/5/"},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/teams/abb/projects/2/sessions/5/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"id":7392,
						"team_slug":"abb",
						"project_session_number":5,
						"status":"open",
						"project":{"id":2,"slug":"2","name":"Web"},
						"reporter":{"email":"r@example.test","display_name":"Reporter"},
						"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
						"url":"https://example.test/checkout",
						"updated_at":"2026-05-01T00:00:00Z",
						"pins":[{"id":44,"number":2,"feedback":"button missing","url":null,"selector":null,"element_info":{},"metadata":{}}]
					}`)
				},
			}),
			wantText:  []string{`"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/"`, `"pins":[`, `"number":2`, "button missing"},
			wantPaths: []string{"/api/teams/abb/projects/2/sessions/5/"},
		},
		{
			name:      "get_pin",
			tool:      "get_pin",
			arguments: map[string]any{"pin": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2", "fields": []string{"console", "network"}},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/teams/abb/projects/2/sessions/5/pins/by-number/2/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"id":44,
						"number":2,
						"feedback":"button still broken",
						"url":null,
						"selector":null,
						"element_info":{},
						"metadata":{},
						"screenshot":null,
						"session_replay":null,
						"voice_note":null,
						"video_recording":null,
						"console":[{"message":"boom"}],
						"network":[{"url":"/api/save"}],
						"events":null
					}`)
				},
			}),
			wantText:   []string{`"number":2`, "button still broken", "boom"},
			wantPaths:  []string{"/api/teams/abb/projects/2/sessions/5/pins/by-number/2/"},
			wantQuery:  map[string]string{"fields": "console_logs,network_logs"},
			pathPrefix: "/api/teams/abb/projects/2/sessions/5/pins/by-number/2/",
		},
		{
			name: "get_pins_partial_failure",
			tool: "get_pins",
			arguments: map[string]any{
				"items": []map[string]any{
					{"pin": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=2", "fields": []string{"console"}},
					{"pin": "https://staging.disbug.us/abb/projects/2/sessions/5/?pin=3", "fields": []string{"console"}},
				},
			},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/teams/abb/projects/2/sessions/5/pins/by-number/2/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"id":44,
						"number":2,
						"feedback":"button still broken",
						"url":null,
						"selector":null,
						"element_info":{},
						"metadata":{},
						"console":[{"message":"boom"}]
					}`)
				},
				"/api/teams/abb/projects/2/sessions/5/pins/by-number/3/": func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"code":"not_found","detail":"pin missing","request_id":"req-missing"}`)
				},
			}),
			wantText:  []string{`"pins":[`, "button still broken", `"errors":[`, "pin missing", "req-missing"},
			wantPaths: []string{"/api/teams/abb/projects/2/sessions/5/pins/by-number/2/", "/api/teams/abb/projects/2/sessions/5/pins/by-number/3/"},
		},
		{
			name:      "search_sessions",
			tool:      "search_sessions",
			arguments: map[string]any{"query": "checkout", "limit": 3},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/search/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"results":[{
							"id":7392,
							"team_slug":"abb",
							"project":{"id":2,"slug":"2","name":"Web"},
							"project_session_number":5,
							"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
							"url":"https://example.test/checkout",
							"status":"open",
							"pin_count":2,
							"first_pin_feedback":"checkout button broken",
							"reporter":{"email":"r@example.test","display_name":"Reporter"},
							"updated_at":"2026-05-01T00:00:00Z",
							"free_tier_locked":false
						}],
						"total":1
					}`)
				},
			}),
			wantText:  []string{`"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/"`, `"total":1`, "checkout button broken"},
			wantPaths: []string{"/api/search/"},
			wantQuery: map[string]string{"q": "checkout", "scope": "sessions", "limit": "3"},
		},
		{
			name:      "search_pins",
			tool:      "search_pins",
			arguments: map[string]any{"query": "checkout", "limit": 4},
			handler: e2eBackendHandler(t, map[string]http.HandlerFunc{
				"/api/search/": func(w http.ResponseWriter, _ *http.Request) {
					writeJSON(t, w, `{
						"results":[{
							"pin":{"id":44,"number":2,"feedback":"checkout button broken","url":null,"selector":"#checkout","element_info":{},"metadata":{}},
							"session":{
								"id":7392,
								"team_slug":"abb",
								"project":{"id":2,"slug":"2","name":"Web"},
								"project_session_number":5,
								"report_url":"https://staging.disbug.us/abb/projects/2/sessions/5/",
								"url":"https://example.test/checkout",
								"status":"open",
								"pin_count":2,
								"first_pin_feedback":"checkout button broken",
								"reporter":{"email":"r@example.test","display_name":"Reporter"},
								"updated_at":"2026-05-01T00:00:00Z",
								"free_tier_locked":false
							}
						}],
						"total":1
					}`)
				},
			}),
			wantText:  []string{`"number":2`, `"total":1`, "checkout button broken"},
			wantPaths: []string{"/api/search/"},
			wantQuery: map[string]string{"q": "checkout", "scope": "pins", "limit": "4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan *http.Request, 16)
			backend := httptest.NewServer(recordRequests(requests, tt.handler))
			t.Cleanup(backend.Close)

			configHome := t.TempDir()
			writeTokenProfile(t, configHome, backend.URL)
			proc := startMCP(t, binary, configHome)
			defer proc.close(t)

			initializeMCP(t, proc)
			text := callToolE2E(t, proc, tt.tool, tt.arguments)
			for _, want := range tt.wantText {
				if !strings.Contains(text, want) {
					t.Fatalf("%s content = %q, want %q", tt.tool, text, want)
				}
			}

			seenRequests := waitForE2ERequests(t, requests, tt.wantPaths)
			for _, path := range tt.wantPaths {
				req := seenRequests[path]
				if got, want := req.Method, http.MethodGet; got != want {
					t.Fatalf("%s method = %q, want %q", path, got, want)
				}
				if got, want := req.Header.Get("Authorization"), "Bearer dba_aaaaaaaaaaaaaaaaaaaaaaaa"; got != want {
					t.Fatalf("%s Authorization = %q, want %q", path, got, want)
				}
				if tt.pathPrefix == "" || path == tt.pathPrefix {
					for key, want := range tt.wantQuery {
						if got := req.URL.Query().Get(key); got != want {
							t.Fatalf("%s query %s = %q, want %q", path, key, got, want)
						}
					}
				}
			}
		})
	}
}

type mcpProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *bytes.Buffer
	done   chan error
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func buildBinary(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	binary := filepath.Join(dir, "disbug")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/disbug")
	cmd.Dir = repoRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build -o %s ./cmd/disbug failed: %v\n%s", binary, err, output)
	}

	return binary
}

func startMCP(t *testing.T, binary, configHome string) *mcpProcess {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), binary, "mcp")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(filteredEnv(), "XDG_CONFIG_HOME="+configHome)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start disbug mcp: %v", err)
	}

	proc := &mcpProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdoutPipe),
		stderr: stderr,
		done:   make(chan error, 1),
	}
	go func() {
		proc.done <- cmd.Wait()
	}()

	return proc
}

func (p *mcpProcess) close(t *testing.T) {
	t.Helper()

	_ = p.stdin.Close()
	select {
	case <-p.done:
	case <-time.After(e2eTimeout):
		_ = p.cmd.Process.Kill()
		err := <-p.done
		t.Fatalf("disbug mcp did not exit after stdin close; kill result: %v; stderr: %s", err, p.stderr.String())
	}
}

func waitForExit(t *testing.T, p *mcpProcess, timeout time.Duration) {
	t.Helper()

	select {
	case err := <-p.done:
		if err != nil {
			t.Fatalf("disbug mcp exited with error: %v; stderr: %s", err, p.stderr.String())
		}
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
		err := <-p.done
		t.Fatalf("disbug mcp did not exit within %s after EOF; kill result: %v; stderr: %s", timeout, err, p.stderr.String())
	}
}

func initializeMCP(t *testing.T, proc *mcpProcess) rpcResponse {
	t.Helper()

	resp := rpcCall(t, proc, 1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "test",
			"version": "0",
		},
	})
	rpcNotify(t, proc, "notifications/initialized", map[string]any{})

	return resp
}

func callToolE2E(t *testing.T, proc *mcpProcess, name string, arguments map[string]any) string {
	t.Helper()

	resp := rpcCall(t, proc, 3, "tools/call", map[string]any{
		"name":      name,
		"arguments": arguments,
	})

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode tools/call result for %s: %v; raw = %s", name, err, resp.Result)
	}
	if result.IsError {
		t.Fatalf("%s returned MCP tool error; result = %s", name, resp.Result)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s returned no content; result = %s", name, resp.Result)
	}
	if result.Content[0].Type != "text" {
		t.Fatalf("%s first content type = %q, want text; result = %s", name, result.Content[0].Type, resp.Result)
	}
	if result.Content[0].Text == "" {
		t.Fatalf("%s first text content is empty; result = %s", name, resp.Result)
	}

	return result.Content[0].Text
}

func rpcCall(t *testing.T, proc *mcpProcess, id int, method string, params any) rpcResponse {
	t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	writeRPC(t, proc, msg)

	resp := readRPCResponse(t, proc)
	if resp.ID != id {
		t.Fatalf("%s response id = %d, want %d; result = %s", method, resp.ID, id, resp.Result)
	}
	if resp.Error != nil {
		t.Fatalf("%s returned JSON-RPC error: code=%d message=%q; stderr: %s", method, resp.Error.Code, resp.Error.Message, proc.stderr.String())
	}

	return resp
}

func rpcNotify(t *testing.T, proc *mcpProcess, method string, params any) {
	t.Helper()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	writeRPC(t, proc, msg)
}

func writeRPC(t *testing.T, proc *mcpProcess, msg map[string]any) {
	t.Helper()

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal JSON-RPC message: %v", err)
	}
	if _, err := fmt.Fprintln(proc.stdin, string(data)); err != nil {
		t.Fatalf("write JSON-RPC message %s: %v; stderr: %s", data, err, proc.stderr.String())
	}
}

func readRPCResponse(t *testing.T, proc *mcpProcess) rpcResponse {
	t.Helper()

	type readResult struct {
		line string
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		line, err := proc.stdout.ReadString('\n')
		ch <- readResult{line: line, err: err}
	}()

	select {
	case result := <-ch:
		if result.err != nil {
			t.Fatalf("read JSON-RPC response: %v; line = %q; stderr: %s", result.err, result.line, proc.stderr.String())
		}
		var resp rpcResponse
		if err := json.Unmarshal([]byte(result.line), &resp); err != nil {
			t.Fatalf("decode JSON-RPC response %q: %v; stderr: %s", result.line, err, proc.stderr.String())
		}
		if resp.JSONRPC != "2.0" {
			t.Fatalf("jsonrpc = %q, want 2.0; line = %s", resp.JSONRPC, result.line)
		}
		return resp
	case <-time.After(e2eTimeout):
		t.Fatalf("timed out waiting for JSON-RPC response; stderr: %s", proc.stderr.String())
	}

	return rpcResponse{}
}

func writeTokenProfile(t *testing.T, configHome, apiURL string) {
	t.Helper()

	dir := filepath.Join(configHome, "disbug")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("create token config dir: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatalf("chmod token config dir: %v", err)
		}
	}

	profile := []byte(fmt.Sprintf(`{"token":"dba_aaaaaaaaaaaaaaaaaaaaaaaa","api_url":%q}`+"\n", apiURL))
	path := filepath.Join(dir, "default.json")
	if err := os.WriteFile(path, profile, 0o600); err != nil {
		t.Fatalf("write token profile: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatalf("chmod token profile: %v", err)
		}
	}
}

func e2eBackendHandler(t *testing.T, routes map[string]http.HandlerFunc) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/me/" {
			writeJSON(t, w, `{
				"agent_name":"a",
				"team":"T",
				"team_slug":"t",
				"api_version":"1.0.0",
				"capabilities":["search","pin_field_selection","scoped_pin_lookup"]
			}`)
			return
		}
		if routes != nil {
			if handler := routes[r.URL.Path]; handler != nil {
				handler(w, r)
				return
			}
		}

		http.Error(w, "unexpected endpoint", http.StatusNotFound)
	}
}

func recordRequests(requests chan<- *http.Request, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		next.ServeHTTP(w, r)
	}
}

func waitForE2ERequests(t *testing.T, requests <-chan *http.Request, paths []string) map[string]*http.Request {
	t.Helper()

	pending := make(map[string]bool, len(paths))
	for _, path := range paths {
		pending[path] = true
	}
	seen := make(map[string]*http.Request, len(paths))

	deadline := time.After(e2eTimeout)
	for len(pending) > 0 {
		select {
		case req := <-requests:
			if pending[req.URL.Path] {
				seen[req.URL.Path] = req
				delete(pending, req.URL.Path)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for requests: %v", pending)
		}
	}

	return seen
}

func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func filteredEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		switch {
		case strings.HasPrefix(entry, "XDG_CONFIG_HOME="):
		case strings.HasPrefix(entry, "DISBUG_TOKEN="):
		case strings.HasPrefix(entry, "DISBUG_API_URL="):
		case strings.HasPrefix(entry, "DISBUG_PROFILE="):
		default:
			filtered = append(filtered, entry)
		}
	}

	return filtered
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
