package supervisor

import "testing"

func TestExtractPluginRuntimeLogFieldsFromJSONLine(t *testing.T) {
	fields := extractPluginRuntimeLogFields(`{"message":"ai craft trace emitted","node_name":"N03_unified_llm","trace_type":"agent-llm-unified-request","session_id":"email-cs-1","request_id":"req-1"}`)

	if got := fields["node_name"]; got != "N03_unified_llm" {
		t.Fatalf("node_name = %v", got)
	}
	if got := fields["trace_type"]; got != "agent-llm-unified-request" {
		t.Fatalf("trace_type = %v", got)
	}
	if got := fields["plugin_message"]; got != "ai craft trace emitted" {
		t.Fatalf("plugin_message = %v", got)
	}
}

func TestExtractPluginRuntimeLogFieldsFromLogrusTextLine(t *testing.T) {
	line := "\x1b[36mINFO\x1b[0m[2026-05-12 00:40:43]logger.go:308 logger.logrusEntryBridge.Info ai craft trace emitted " +
		"\x1b[36mnode_name\x1b[0m=N02_context_build " +
		"\x1b[36mnode_seq\x1b[0m=2 " +
		"\x1b[36mtrace_type\x1b[0m=agent-llm-slot-state " +
		"\x1b[36msession_id\x1b[0m=email-cs-56f44eec541e490b " +
		"\x1b[36muser_text\x1b[0m=\"你好\""

	fields := extractPluginRuntimeLogFields(line)
	if got := fields["node_name"]; got != "N02_context_build" {
		t.Fatalf("node_name = %v; fields=%#v", got, fields)
	}
	if got := fields["node_seq"]; got != "2" {
		t.Fatalf("node_seq = %v; fields=%#v", got, fields)
	}
	if got := fields["trace_type"]; got != "agent-llm-slot-state" {
		t.Fatalf("trace_type = %v; fields=%#v", got, fields)
	}
	if got := fields["session_id"]; got != "email-cs-56f44eec541e490b" {
		t.Fatalf("session_id = %v; fields=%#v", got, fields)
	}
	if _, exists := fields["user_text"]; exists {
		t.Fatalf("user_text should not be promoted to runtime log top-level: %#v", fields)
	}
}
