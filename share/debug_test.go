package share

import "testing"

func TestDebugEnabled(t *testing.T) {
	t.Setenv(DebugEnv, "")
	if DebugEnabled() {
		t.Fatal("debug should be disabled for empty env")
	}

	t.Setenv(DebugEnv, "0")
	if DebugEnabled() {
		t.Fatal("debug should be disabled for 0")
	}

	t.Setenv(DebugEnv, "false")
	if DebugEnabled() {
		t.Fatal("debug should be disabled for false")
	}

	t.Setenv(DebugEnv, "1")
	if !DebugEnabled() {
		t.Fatal("debug should be enabled for 1")
	}
}
