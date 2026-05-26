//go:build !windows

package tools

func discoverGitHubTokenFromSystemCredentialStore(_ copilotProfile) (string, string) {
	return "", ""
}
