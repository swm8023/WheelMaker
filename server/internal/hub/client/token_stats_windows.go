//go:build windows

package client

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

const credTypeGeneric uint32 = 1

type winCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	modAdvapi32   = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW = modAdvapi32.NewProc("CredReadW")
	procCredFree  = modAdvapi32.NewProc("CredFree")
)

func discoverGitHubTokenFromSystemCredentialStore(profile copilotProfile) (string, string) {
	targets := candidateCopilotCredentialTargets(profile)
	for _, target := range targets {
		blob, err := readGenericCredentialBlob(target)
		if err != nil || len(blob) == 0 {
			continue
		}
		if token := normalizeCredentialToken(blob); token != "" {
			return token, "wincred:" + target
		}
	}
	return "", ""
}

func candidateCopilotCredentialTargets(profile copilotProfile) []string {
	alias := strings.TrimSpace(profile.Alias)
	if alias == "" || strings.EqualFold(alias, "current") {
		return []string{
			"copilot-cli/https://github.com",
			"copilot-cli/github.com",
			"copilot-cli/https://github.com:current",
			"copilot-cli/github.com:current",
		}
	}
	host := strings.TrimSpace(profile.Host)
	if host == "" {
		host = "https://github.com"
	}
	hostNoScheme := strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	candidates := []string{
		"copilot-cli/" + host + ":" + alias,
		"copilot-cli/" + hostNoScheme + ":" + alias,
		"copilot-cli/" + host,
		"copilot-cli/" + hostNoScheme,
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, item := range candidates {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func readGenericCredentialBlob(target string) ([]byte, error) {
	targetPtr, err := windows.UTF16PtrFromString(strings.TrimSpace(target))
	if err != nil {
		return nil, err
	}
	var pcred uintptr
	ret, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		uintptr(credTypeGeneric),
		0,
		uintptr(unsafe.Pointer(&pcred)),
	)
	if ret == 0 {
		if callErr != nil && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return nil, callErr
		}
		return nil, windows.ERROR_NOT_FOUND
	}
	defer procCredFree.Call(pcred)
	cred := (*winCredential)(unsafe.Pointer(pcred))
	if cred == nil || cred.CredentialBlob == nil || cred.CredentialBlobSize == 0 {
		return nil, nil
	}
	size := int(cred.CredentialBlobSize)
	blob := unsafe.Slice(cred.CredentialBlob, size)
	out := make([]byte, size)
	copy(out, blob)
	return out, nil
}

func normalizeCredentialToken(blob []byte) string {
	utf8Token := strings.TrimSpace(strings.Trim(string(blob), "\x00"))
	if isLikelyOAuthToken(utf8Token) {
		return utf8Token
	}
	if len(blob)%2 == 0 {
		u16 := make([]uint16, 0, len(blob)/2)
		for i := 0; i+1 < len(blob); i += 2 {
			v := uint16(blob[i]) | uint16(blob[i+1])<<8
			if v == 0 {
				break
			}
			u16 = append(u16, v)
		}
		if len(u16) > 0 {
			utf16Token := strings.TrimSpace(string(utf16.Decode(u16)))
			if isLikelyOAuthToken(utf16Token) {
				return utf16Token
			}
		}
	}
	return ""
}

func isLikelyOAuthToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if strings.ContainsAny(token, " \t\r\n") {
		return false
	}
	// GitHub fine-grained and OAuth token prefixes observed in Copilot flows.
	for _, prefix := range []string{"gho_", "github_pat_", "ghu_"} {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	// Fallback for unknown but plausible bearer tokens.
	return len(token) >= 32
}
