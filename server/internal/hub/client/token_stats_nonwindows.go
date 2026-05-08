//go:build !windows

package client

func discoverGitHubTokenFromSystemCredentialStore(_ copilotProfile) (string, string) {
	return "", ""
}
