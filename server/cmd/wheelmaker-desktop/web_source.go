package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	desktopWebSourcePreferenceAuto     = "auto"
	desktopWebSourcePreferenceEmbedded = "embedded"

	desktopWebSourceActualEmbedded = "embedded"
	desktopWebSourceActualRemote   = "remote"
)

type desktopWebSourceConfig struct {
	WebSourcePreference     string `json:"webSourcePreference"`
	RemoteWebURL            string `json:"remoteWebUrl,omitempty"`
	RemoteWebRegistryOrigin string `json:"remoteWebRegistryOrigin,omitempty"`
}

type desktopRemoteWebCandidate struct {
	Source          string `json:"source"`
	RegistryAddress string `json:"registryAddress"`
	RemoteWebURL    string `json:"remoteWebUrl"`
}

type desktopWebSourceState struct {
	Preference    string `json:"preference"`
	ActualSource  string `json:"actualSource"`
	DisplayTitle  string `json:"displayTitle"`
	DisplaySource string `json:"displaySource"`
	RemoteURL     string `json:"remoteUrl"`
	RemoteHost    string `json:"remoteHost"`
}

type desktopWebSourceConfigStore interface {
	Load() (desktopWebSourceConfig, error)
	Save(desktopWebSourceConfig) error
}

type desktopFileWebSourceConfigStore struct {
	path string
}

func newDefaultDesktopWebSourceConfigStore() desktopWebSourceConfigStore {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return desktopFileWebSourceConfigStore{path: "desktop-config.json"}
	}
	return desktopFileWebSourceConfigStore{
		path: filepath.Join(home, ".wheelmaker", "desktop", "config.json"),
	}
}

func (s desktopFileWebSourceConfigStore) Load() (desktopWebSourceConfig, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return defaultDesktopWebSourceConfig(), err
	}
	var config desktopWebSourceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return defaultDesktopWebSourceConfig(), err
	}
	return sanitizeDesktopWebSourceConfig(config), nil
}

func (s desktopFileWebSourceConfigStore) Save(config desktopWebSourceConfig) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sanitizeDesktopWebSourceConfig(config), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path, data, 0o600)
}

type desktopWebSourceRuntime struct {
	mu              sync.RWMutex
	store           desktopWebSourceConfigStore
	client          *http.Client
	config          desktopWebSourceConfig
	actual          string
	actualRemoteURL string
}

func newEmbeddedOnlyDesktopWebSourceRuntime() *desktopWebSourceRuntime {
	return newDesktopWebSourceRuntime(&staticDesktopWebSourceConfigStore{
		config: desktopWebSourceConfig{WebSourcePreference: desktopWebSourcePreferenceEmbedded},
	}, nil)
}

func newDefaultDesktopWebSourceRuntime() *desktopWebSourceRuntime {
	return newDesktopWebSourceRuntime(newDefaultDesktopWebSourceConfigStore(), nil)
}

func newDesktopWebSourceRuntime(store desktopWebSourceConfigStore, client *http.Client) *desktopWebSourceRuntime {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	config, err := store.Load()
	if err != nil {
		config = defaultDesktopWebSourceConfig()
	}
	config = sanitizeDesktopWebSourceConfig(config)
	runtime := &desktopWebSourceRuntime{
		store:  store,
		client: client,
		config: config,
		actual: actualDesktopWebSourceForConfig(config),
	}
	if runtime.actual == desktopWebSourceActualRemote {
		runtime.actualRemoteURL = config.RemoteWebURL
	}
	return runtime
}

func (r *desktopWebSourceRuntime) State() desktopWebSourceState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return desktopWebSourceStateForConfig(r.config, r.actual, r.actualRemoteURL)
}

func (r *desktopWebSourceRuntime) SetPreference(preference string) (desktopWebSourceState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if preference != desktopWebSourcePreferenceEmbedded {
		preference = desktopWebSourcePreferenceAuto
	}
	r.config.WebSourcePreference = preference
	r.actual = actualDesktopWebSourceForConfig(r.config)
	if r.actual == desktopWebSourceActualRemote {
		r.actualRemoteURL = r.config.RemoteWebURL
	} else {
		r.actualRemoteURL = ""
	}
	if err := r.store.Save(r.config); err != nil {
		return desktopWebSourceState{}, err
	}
	return desktopWebSourceStateForConfig(r.config, r.actual, r.actualRemoteURL), nil
}

func (r *desktopWebSourceRuntime) SetRemoteCandidate(candidate desktopRemoteWebCandidate) (desktopWebSourceState, error) {
	nextRemoteURL, registryOrigin, ok := normalizeDesktopRemoteWebCandidate(candidate)

	r.mu.Lock()
	defer r.mu.Unlock()
	if ok {
		r.config.RemoteWebURL = nextRemoteURL
		r.config.RemoteWebRegistryOrigin = registryOrigin
	} else {
		r.config.RemoteWebURL = ""
		r.config.RemoteWebRegistryOrigin = ""
	}
	if err := r.store.Save(r.config); err != nil {
		return desktopWebSourceState{}, err
	}
	return desktopWebSourceStateForConfig(r.config, r.actual, r.actualRemoteURL), nil
}

func (r *desktopWebSourceRuntime) SetActualSource(actual string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if actual != desktopWebSourceActualRemote {
		actual = desktopWebSourceActualEmbedded
	}
	r.actual = actual
	if actual == desktopWebSourceActualRemote {
		r.actualRemoteURL = r.config.RemoteWebURL
	} else {
		r.actualRemoteURL = ""
	}
}

func (r *desktopWebSourceRuntime) RefreshActualSource() desktopWebSourceState {
	remoteBase, ok := r.remoteBaseURL()
	if !ok {
		r.SetActualSource(desktopWebSourceActualEmbedded)
		return r.State()
	}
	remoteURL, ok := buildDesktopRemoteAssetURL(remoteBase, "index.html")
	if !ok {
		r.SetActualSource(desktopWebSourceActualEmbedded)
		return r.State()
	}
	resp, err := r.httpClient().Get(remoteURL)
	if err != nil {
		r.SetActualSource(desktopWebSourceActualEmbedded)
		return r.State()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified || (resp.StatusCode >= 200 && resp.StatusCode <= 299) {
		r.SetActualSource(desktopWebSourceActualRemote)
		return r.State()
	}
	r.SetActualSource(desktopWebSourceActualEmbedded)
	return r.State()
}

func (r *desktopWebSourceRuntime) remoteBaseURL() (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.config.WebSourcePreference != desktopWebSourcePreferenceAuto || r.actual != desktopWebSourceActualRemote || r.actualRemoteURL == "" {
		return "", false
	}
	return r.actualRemoteURL, true
}

func (r *desktopWebSourceRuntime) httpClient() *http.Client {
	if r.client != nil {
		return r.client
	}
	return http.DefaultClient
}

func (r *desktopWebSourceRuntime) WindowTitle() string {
	return r.State().DisplayTitle
}

type staticDesktopWebSourceConfigStore struct {
	config desktopWebSourceConfig
}

func (s *staticDesktopWebSourceConfigStore) Load() (desktopWebSourceConfig, error) {
	return s.config, nil
}

func (s *staticDesktopWebSourceConfigStore) Save(config desktopWebSourceConfig) error {
	s.config = config
	return nil
}

func defaultDesktopWebSourceConfig() desktopWebSourceConfig {
	return desktopWebSourceConfig{WebSourcePreference: desktopWebSourcePreferenceAuto}
}

func sanitizeDesktopWebSourceConfig(config desktopWebSourceConfig) desktopWebSourceConfig {
	if config.WebSourcePreference != desktopWebSourcePreferenceEmbedded {
		config.WebSourcePreference = desktopWebSourcePreferenceAuto
	}
	if config.RemoteWebURL != "" {
		if remoteURL, ok := normalizeDesktopRemoteWebURL(config.RemoteWebURL); ok {
			config.RemoteWebURL = remoteURL
		} else {
			config.RemoteWebURL = ""
			config.RemoteWebRegistryOrigin = ""
		}
	}
	return config
}

func actualDesktopWebSourceForConfig(config desktopWebSourceConfig) string {
	if config.WebSourcePreference == desktopWebSourcePreferenceAuto && config.RemoteWebURL != "" {
		return desktopWebSourceActualRemote
	}
	return desktopWebSourceActualEmbedded
}

func desktopWebSourceStateForConfig(config desktopWebSourceConfig, actual string, actualRemoteURL string) desktopWebSourceState {
	remoteHost := ""
	if config.RemoteWebURL != "" {
		if parsed, err := url.Parse(config.RemoteWebURL); err == nil {
			remoteHost = parsed.Host
		}
	}
	displaySource := "Embedded"
	if actual == desktopWebSourceActualRemote && actualRemoteURL != "" {
		if parsed, err := url.Parse(actualRemoteURL); err == nil && parsed.Host != "" {
			displaySource = parsed.Host
		} else {
			actual = desktopWebSourceActualEmbedded
		}
	} else {
		actual = desktopWebSourceActualEmbedded
	}
	return desktopWebSourceState{
		Preference:    config.WebSourcePreference,
		ActualSource:  actual,
		DisplayTitle:  "WheelMaker - " + displaySource,
		DisplaySource: displaySource,
		RemoteURL:     config.RemoteWebURL,
		RemoteHost:    remoteHost,
	}
}

func normalizeDesktopRemoteWebCandidate(candidate desktopRemoteWebCandidate) (string, string, bool) {
	registryURL, ok := normalizeDesktopRegistryURL(candidate.RegistryAddress)
	if !ok {
		return "", "", false
	}
	remoteURL, ok := normalizeDesktopRemoteWebURL(candidate.RemoteWebURL)
	if !ok {
		return "", "", false
	}
	parsedRemote, _ := url.Parse(remoteURL)
	if !strings.EqualFold(parsedRemote.Host, registryURL.Host) {
		return "", "", false
	}
	return remoteURL, "wss://" + registryURL.Host, true
}

func normalizeDesktopRegistryURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return nil, false
	}
	switch parsed.Scheme {
	case "wss", "https":
	default:
		return nil, false
	}
	if isDesktopLoopbackHost(parsed.Hostname()) {
		return nil, false
	}
	return parsed, true
}

func normalizeDesktopRemoteWebURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", false
	}
	if isDesktopLoopbackHost(parsed.Hostname()) {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	return "https://" + parsed.Host + "/", true
}

func isDesktopLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
