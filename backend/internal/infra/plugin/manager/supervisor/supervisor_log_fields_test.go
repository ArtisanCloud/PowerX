package supervisor

import "testing"

func TestExtractPluginRuntimeLogFieldsFromJSONLine(t *testing.T) {
	fields := extractPluginRuntimeLogFields(`{"message":"plugin runtime trace emitted","node_name":"N03_task_run","trace_type":"plugin-task-request","session_id":"session-1","request_id":"req-1"}`)

	if got := fields["node_name"]; got != "N03_task_run" {
		t.Fatalf("node_name = %v", got)
	}
	if got := fields["trace_type"]; got != "plugin-task-request" {
		t.Fatalf("trace_type = %v", got)
	}
	if got := fields["plugin_message"]; got != "plugin runtime trace emitted" {
		t.Fatalf("plugin_message = %v", got)
	}
}

func TestExtractPluginRuntimeLogFieldsFromLogrusTextLine(t *testing.T) {
	line := "\x1b[36mINFO\x1b[0m[2026-05-12 00:40:43]logger.go:308 logger.logrusEntryBridge.Info plugin runtime trace emitted " +
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

func TestAllowForwardToStdIOFollowsGlobalConsoleConfig(t *testing.T) {
	t.Setenv("POWERX_SUPERVISOR_FORWARD_STDIO", "")

	prev := globalConsoleEnabled
	t.Cleanup(func() {
		globalConsoleEnabled = prev
	})

	globalConsoleEnabled = func() bool { return false }
	if allowForwardToStdIO() {
		t.Fatalf("allowForwardToStdIO() = true, want false when log.console=false")
	}

	globalConsoleEnabled = func() bool { return true }
	if !allowForwardToStdIO() {
		t.Fatalf("allowForwardToStdIO() = false, want true when log.console=true")
	}
}

func TestAllowForwardToStdIOEnvOverridesGlobalConsoleConfig(t *testing.T) {
	prev := globalConsoleEnabled
	t.Cleanup(func() {
		globalConsoleEnabled = prev
	})

	globalConsoleEnabled = func() bool { return false }
	t.Setenv("POWERX_SUPERVISOR_FORWARD_STDIO", "true")
	if !allowForwardToStdIO() {
		t.Fatalf("allowForwardToStdIO() = false, want true when env override is true")
	}

	globalConsoleEnabled = func() bool { return true }
	t.Setenv("POWERX_SUPERVISOR_FORWARD_STDIO", "false")
	if allowForwardToStdIO() {
		t.Fatalf("allowForwardToStdIO() = true, want false when env override is false")
	}
}

func TestMapToEnvKeepsSingleOverriddenKey(t *testing.T) {
	base := envToMap([]string{
		"PX_GATEWAY_BASE_URL=http://old.example",
		"PX_GATEWAY_AUTH_SCHEME=apikey",
	})
	base["PX_GATEWAY_BASE_URL"] = "http://new.example"
	base["PX_GATEWAY_AUTH_SCHEME"] = "bearer"

	env := mapToEnv(base)

	countBaseURL := 0
	countScheme := 0
	for _, item := range env {
		switch item {
		case "PX_GATEWAY_BASE_URL=http://new.example":
			countBaseURL++
		case "PX_GATEWAY_AUTH_SCHEME=bearer":
			countScheme++
		case "PX_GATEWAY_BASE_URL=http://old.example", "PX_GATEWAY_AUTH_SCHEME=apikey":
			t.Fatalf("mapToEnv kept stale env value %q in %#v", item, env)
		}
	}
	if countBaseURL != 1 {
		t.Fatalf("PX_GATEWAY_BASE_URL count = %d, want 1; env=%#v", countBaseURL, env)
	}
	if countScheme != 1 {
		t.Fatalf("PX_GATEWAY_AUTH_SCHEME count = %d, want 1; env=%#v", countScheme, env)
	}
}
