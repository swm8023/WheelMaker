package client

import (
	"testing"

	"github.com/swm8023/wheelmaker/internal/acp"
)

func TestIsAgentExitError(t *testing.T) {
	cases := []string{
		"acp rpc error -1: agent process exited",
		"io: broken pipe",
		"read tcp ... connection reset by peer",
		"EOF",
	}
	for _, c := range cases {
		if !isAgentExitError(assertErr(c)) {
			t.Fatalf("expected agent-exit match for %q", c)
		}
	}
}

func TestIsAgentExitErrorFalse(t *testing.T) {
	if isAgentExitError(assertErr("Selected model is at capacity")) {
		t.Fatalf("capacity error must not be treated as process exit")
	}
}

func TestHasSandboxRefreshError(t *testing.T) {
	u := acp.Update{
		Type:    acp.UpdateToolCall,
		Content: "tool failed: windows sandbox: spawn setup refresh",
	}
	if !hasSandboxRefreshError(u) {
		t.Fatal("expected sandbox refresh detection")
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

func assertErr(s string) error { return testErr(s) }
