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
		"/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/": {
			"GET": "GetSession",
		},
		"/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/status/": {
			"POST": "SetSessionStatus",
		},
		"/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/pins/by-number/{pin_number}/": {
			"GET": "GetPinByNumber",
		},
		"/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/pins/by-number/{pin_number}/attachments/{attachment_id}/download/": {
			"GET": "DownloadAttachment",
		},
		"/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/pins/by-number/{pin_number}/status/": {
			"POST": "SetPinStatus",
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
		"Attachment",
		"AgentActivity",
		"ConsoleLogItem",
		"ErrorEnvelope",
		"ListSessionsResponse",
		"Me",
		"NetworkLogItem",
		"PinFull",
		"PinLite",
		"PinStatusResponse",
		"Project",
		"Reporter",
		"SearchPinsResponse",
		"SearchSessionsResponse",
		"SessionDetail",
		"SessionAttachment",
		"SessionStatusResponse",
		"SessionSummary",
		"StatusUpdate",
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

	pinByNumber := doc.Paths.Find("/api/teams/{team_slug}/projects/{project_id}/sessions/{session_number}/pins/by-number/{pin_number}/").Get
	fieldsParam := operationParameter(pinByNumber, "fields")
	if fieldsParam == nil {
		t.Fatal("GET scoped pin-by-number fields parameter is missing")
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

	assertSchemaType(t, doc.Components.Parameters["ProjectID"].Value.Schema.Value, "integer")
	assertSchemaType(t, doc.Components.Parameters["SessionNumber"].Value.Schema.Value, "integer")
	assertPropertyAbsent(t, doc.Components.Schemas["SessionSummary"].Value, "id")
	assertPropertyAbsent(t, doc.Components.Schemas["SessionDetail"].Value, "id")
	assertPropertyAbsent(t, doc.Components.Schemas["PinLite"].Value, "id")
	assertSchemaType(t, doc.Components.Schemas["SessionSummary"].Value.Properties["project_session_number"].Value, "integer")
	assertSchemaType(t, doc.Components.Schemas["SessionSummary"].Value.Properties["report_url"].Value, "string")
	assertSchemaType(t, doc.Components.Schemas["PinLite"].Value.Properties["number"].Value, "integer")
	assertPropertyPresent(t, doc.Components.Schemas["PinLite"].Value, "status")
	assertPropertyPresent(t, doc.Components.Schemas["SessionDetail"].Value, "agent_log")
	assertPropertyNullable(t, doc.Components.Schemas["SessionSummary"].Value, "project")
	assertPropertyNullable(t, doc.Components.Schemas["SessionSummary"].Value, "reporter")
	assertPropertyPresent(t, doc.Components.Schemas["SessionSummary"].Value, "title")
	assertPropertyPresent(t, doc.Components.Schemas["SessionSummary"].Value, "attachments")
	assertPropertyPresent(t, doc.Components.Schemas["SessionDetail"].Value, "title")
	assertPropertyPresent(t, doc.Components.Schemas["PinLite"].Value, "attachments")
	assertPropertyAbsent(t, doc.Components.Schemas["SessionSummary"].Value, "created_at")
	assertPropertyAbsent(t, doc.Components.Schemas["ErrorEnvelope"].Value, "error")
	assertPropertyPresent(t, doc.Components.Schemas["ErrorEnvelope"].Value, "code")
	assertPropertyPresent(t, doc.Components.Schemas["ErrorEnvelope"].Value, "detail")
	assertPropertyPresent(t, doc.Components.Schemas["ErrorEnvelope"].Value, "request_id")
	assertPropertyPresent(t, doc.Components.Schemas["Me"].Value, "agent_name")
	assertPropertyPresent(t, doc.Components.Schemas["Me"].Value, "created_by_email")
	assertPropertyAbsent(t, doc.Components.Schemas["Me"].Value, "team_id")

	pinFullExtension := doc.Components.Schemas["PinFull"].Value.AllOf[1].Value
	assertPropertyPresent(t, pinFullExtension, "console")
	assertPropertyPresent(t, pinFullExtension, "network")
	assertPropertyPresent(t, pinFullExtension, "events")
	assertPropertyAbsent(t, pinFullExtension, "console_logs")
	assertPropertyAbsent(t, pinFullExtension, "network_logs")
	assertPropertyAbsent(t, pinFullExtension, "user_events")
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

func assertSchemaType(t *testing.T, schema *openapi3.Schema, want string) {
	t.Helper()
	if schema == nil || schema.Type == nil || !schema.Type.Is(want) {
		t.Fatalf("schema type = %v, want %q", schema.Type, want)
	}
}

func assertPropertyPresent(t *testing.T, schema *openapi3.Schema, name string) {
	t.Helper()
	if schema.Properties[name] == nil {
		t.Fatalf("schema property %q is missing", name)
	}
}

func assertPropertyAbsent(t *testing.T, schema *openapi3.Schema, name string) {
	t.Helper()
	if schema.Properties[name] != nil {
		t.Fatalf("schema property %q is present, want absent", name)
	}
}

func assertPropertyNullable(t *testing.T, schema *openapi3.Schema, name string) {
	t.Helper()
	property := schema.Properties[name]
	if property == nil || property.Value == nil {
		t.Fatalf("schema property %q is missing", name)
	}
	if !property.Value.Nullable {
		t.Fatalf("schema property %q nullable = false, want true", name)
	}
}
