package acp_test

// testmain_test.go provides TestMain for the acp_test package.
// When GO_ACP_MOCK=1 is set, this binary acts as the mock ACP server for conn_test.go.
// Otherwise it runs the test suite and sets mockAgentBin for newMockConn.

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_ACP_MOCK") == "1" {
		runConnMockAgent()
		os.Exit(0)
	}
	mockAgentBin = os.Args[0]
	os.Exit(m.Run())
}
