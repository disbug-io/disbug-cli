//go:build schema

package client

import (
	"slices"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestBackendOpenAPISchemaContract(t *testing.T) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("testdata/schema.yaml")
	if err != nil {
		t.Fatalf("load schema fixture: %v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("validate schema fixture: %v", err)
	}

	requiredOperations := map[string]map[string]string{
		"/api/me/": {
			"GET": "GetMe",
		},
		"/api/agent/revoke/": {
			"POST": "RevokeToken",
		},
		"/api/sessions/": {
			"GET": "ListSessions",
		},
		"/api/sessions/{id}/": {
			"GET": "GetSession",
		},
		"/api/sessions/{id}/pins/by-number/{n}/": {
			"GET": "GetPinByNumber",
		},
		"/api/search/": {
			"GET": "Search",
		},
	}
	for path, methods := range requiredOperations {
		pathItem := doc.Paths.Find(path)
		if pathItem == nil {
			t.Fatalf("path %q is missing", path)
		}
		for method, wantOperationID := range methods {
			operation := pathItem.GetOperation(method)
			if operation == nil {
				t.Fatalf("%s %s is missing", method, path)
			}
			if operation.OperationID != wantOperationID {
				t.Fatalf("%s %s operationId = %q, want %q", method, path, operation.OperationID, wantOperationID)
			}
		}
	}

	requiredSchemas := []string{
		"Asset",
		"ConsoleLogItem",
		"ErrorEnvelope",
		"ListSessionsResponse",
		"Me",
		"NetworkLogItem",
		"PinFull",
		"PinLite",
		"Project",
		"Reporter",
		"SearchPinsResponse",
		"SearchSessionsResponse",
		"SessionDetail",
		"SessionSummary",
		"UserEventItem",
	}
	for _, name := range requiredSchemas {
		if doc.Components.Schemas[name] == nil {
			t.Fatalf("component schema %q is missing", name)
		}
	}

	search := doc.Paths.Find("/api/search/").Get
	qParam := operationParameter(search, "q")
	if qParam == nil {
		t.Fatal("GET /api/search/ q parameter is missing")
	}
	if !qParam.Required {
		t.Fatal("GET /api/search/ q parameter must be required")
	}

	pinByNumber := doc.Paths.Find("/api/sessions/{id}/pins/by-number/{n}/").Get
	fieldsParam := operationParameter(pinByNumber, "fields")
	if fieldsParam == nil {
		t.Fatal("GET /api/sessions/{id}/pins/by-number/{n}/ fields parameter is missing")
	}
	if fieldsParam.Schema == nil || fieldsParam.Schema.Value == nil {
		t.Fatal("fields parameter schema is missing")
	}
	requiredFields := []string{
		"console_logs",
		"network_logs",
		"user_events",
		"session_replay",
		"voice_note",
		"video_recording",
		"screenshot",
	}
	for _, want := range requiredFields {
		if !enumContains(fieldsParam.Schema.Value.Enum, want) {
			t.Fatalf("fields parameter enum is missing %q", want)
		}
	}

	// TODO(Phase 2): expand this gate with client round-trip assertions once the
	// real client methods exist for New, Me, ListSessions, GetSession,
	// GetPinByNumber, and Search. This test intentionally avoids fake client
	// production code during the schema-only phase.
}

func operationParameter(operation *openapi3.Operation, name string) *openapi3.Parameter {
	for _, ref := range operation.Parameters {
		if ref.Value != nil && ref.Value.Name == name {
			return ref.Value
		}
	}
	return nil
}

func enumContains(enum []any, want string) bool {
	return slices.ContainsFunc(enum, func(value any) bool {
		got, ok := value.(string)
		return ok && got == want
	})
}
