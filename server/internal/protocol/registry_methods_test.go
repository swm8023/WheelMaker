package protocol

import "testing"

func TestRegistryMethodDescriptorsAreSelfConsistent(t *testing.T) {
	for key, desc := range RegistryMethodDescriptors {
		if key == "" {
			t.Fatal("registry method descriptor has empty key")
		}
		if desc.Method != key {
			t.Fatalf("descriptor key=%q method=%q, want same value", key, desc.Method)
		}
		if desc.Route == "" {
			t.Fatalf("descriptor %q has empty route", key)
		}
	}
}

func TestRegistryMethodRolesAndRoutes(t *testing.T) {
	if !RegistryMethodAllowed(string(RegistryRoleHub), RegistryMethodRegistryReportProjects) {
		t.Fatal("hub should be allowed to report projects")
	}
	if RegistryMethodAllowed(string(RegistryRoleClient), RegistryMethodRegistryReportProjects) {
		t.Fatal("client should not be allowed to report projects")
	}
	if !RegistryClientForwardMethod(RegistryMethodSessionSend) {
		t.Fatal("session.send should be a client forward method")
	}
	if !RegistryHubCommandMethod(RegistryMethodCmdSkills) {
		t.Fatal("cmd.skills should be a hub command method")
	}
	if !RegistryLocalReadMethodAllowed(RegistryMethodFSRead) {
		t.Fatal("fs.read should be allowed on local read")
	}
	if RegistryLocalReadMethodAllowed(RegistryMethodSessionList) {
		t.Fatal("session.list should not be allowed on local read")
	}
}

func TestRegistryHubSessionEventMapping(t *testing.T) {
	method, ok := RegistryHubSessionEventMethod(RegistryMethodRegistrySessionMessage)
	if !ok {
		t.Fatal("registry.session.message should map to a client event")
	}
	if method != RegistryMethodSessionMessage {
		t.Fatalf("client event method=%q, want %q", method, RegistryMethodSessionMessage)
	}
}
