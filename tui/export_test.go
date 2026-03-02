package tui_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	tapv1 "github.com/mickamy/http-tap/gen/tap/v1"
	"github.com/mickamy/http-tap/tui"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func makeEvent(method, path string, status int32, dur time.Duration, errMsg string) *tapv1.HTTPEvent {
	return &tapv1.HTTPEvent{
		Id:        method + "-" + path,
		Method:    method,
		Path:      path,
		Status:    status,
		StartTime: timestamppb.New(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)),
		Duration:  durationpb.New(dur),
		Error:     errMsg,
	}
}

func TestWriteExport_JSON(t *testing.T) {
	t.Parallel()

	events := []*tapv1.HTTPEvent{
		makeEvent("GET", "/api/users", 200, 10*time.Millisecond, ""),
		makeEvent("POST", "/api/users", 201, 5*time.Millisecond, ""),
		makeEvent("GET", "/api/users/1", 500, 100*time.Millisecond, "internal error"),
	}

	dir := t.TempDir()
	path, err := tui.WriteExport(events, "", false, tui.ExportJSON, dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Captured  int `json:"captured"`
		Exported  int `json:"exported"`
		Calls     []json.RawMessage
		Analytics []json.RawMessage
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.Captured != 3 {
		t.Errorf("captured = %d, want 3", result.Captured)
	}
	if result.Exported != 3 {
		t.Errorf("exported = %d, want 3", result.Exported)
	}
	if len(result.Calls) != 3 {
		t.Errorf("calls = %d, want 3", len(result.Calls))
	}
	if len(result.Analytics) == 0 {
		t.Error("analytics should not be empty")
	}
}

func TestWriteExport_JSON_WithFilter(t *testing.T) {
	t.Parallel()

	events := []*tapv1.HTTPEvent{
		makeEvent("GET", "/api/users", 200, 10*time.Millisecond, ""),
		makeEvent("POST", "/api/users", 201, 5*time.Millisecond, ""),
		makeEvent("GET", "/api/orders", 200, 20*time.Millisecond, ""),
	}

	dir := t.TempDir()
	path, err := tui.WriteExport(events, "orders", false, tui.ExportJSON, dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Captured int `json:"captured"`
		Exported int `json:"exported"`
		Search   string
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.Captured != 3 {
		t.Errorf("captured = %d, want 3", result.Captured)
	}
	if result.Exported != 1 {
		t.Errorf("exported = %d, want 1", result.Exported)
	}
	if result.Search != "orders" {
		t.Errorf("search = %q, want %q", result.Search, "orders")
	}
}

func TestWriteExport_JSON_ErrorsOnly(t *testing.T) {
	t.Parallel()

	events := []*tapv1.HTTPEvent{
		makeEvent("GET", "/api/users", 200, 10*time.Millisecond, ""),
		makeEvent("GET", "/api/users/1", 404, 5*time.Millisecond, ""),
		makeEvent("POST", "/api/users", 500, 100*time.Millisecond, "server error"),
	}

	dir := t.TempDir()
	path, err := tui.WriteExport(events, "", true, tui.ExportJSON, dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var result struct {
		Exported int `json:"exported"`
		Calls    []struct {
			Status int32  `json:"status"`
			Error  string `json:"error"`
		}
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.Exported != 2 {
		t.Errorf("exported = %d, want 2", result.Exported)
	}
	for _, c := range result.Calls {
		if c.Status < 400 && c.Error == "" {
			t.Errorf("non-error call included: status=%d", c.Status)
		}
	}
}

func TestWriteExport_Markdown(t *testing.T) {
	t.Parallel()

	events := []*tapv1.HTTPEvent{
		makeEvent("GET", "/api/users", 200, 10*time.Millisecond, ""),
		makeEvent("DELETE", "/api/users/1", 204, 3*time.Millisecond, ""),
	}

	dir := t.TempDir()
	path, err := tui.WriteExport(events, "", false, tui.ExportMarkdown, dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	md := string(data)
	if !strings.Contains(md, "# http-tap export") {
		t.Error("missing markdown header")
	}
	if !strings.Contains(md, "## Requests") {
		t.Error("missing Requests section")
	}
	if !strings.Contains(md, "## Analytics") {
		t.Error("missing Analytics section")
	}
	if !strings.Contains(md, "/api/users") {
		t.Error("missing path in markdown")
	}
}
