package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type deepSeekBalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"totalBalance"`
	GrantedBalance  string `json:"grantedBalance"`
	ToppedUpBalance string `json:"toppedUpBalance"`
}

type deepSeekBalanceView struct {
	IsAvailable bool                  `json:"isAvailable"`
	Items       []deepSeekBalanceInfo `json:"items"`
}

type deepSeekUsageRow struct {
	Bucket       string  `json:"bucket"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	TotalTokens  int64   `json:"totalTokens"`
	Cost         float64 `json:"cost"`
}

type deepSeekUsageView struct {
	RangeType string             `json:"rangeType"`
	Month     string             `json:"month"`
	Rows      []deepSeekUsageRow `json:"rows"`
}

type deepSeekTokenStatsPayload struct {
	OK               bool                `json:"ok"`
	Provider         string              `json:"provider"`
	RangeType        string              `json:"rangeType"`
	Month            string              `json:"month"`
	UpdatedAt        string              `json:"updatedAt"`
	Balance          deepSeekBalanceView `json:"balance"`
	Usage            deepSeekUsageView   `json:"usage"`
	UsageUnavailable bool                `json:"usageUnavailable"`
	UsageMessage     string              `json:"usageMessage,omitempty"`
}

type deepSeekBalanceResponse struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}
type tokenProviderScanResult struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Accounts []tokenProviderAccount `json:"accounts"`
}

type tokenProviderAccount struct {
	ID                       string              `json:"id"`
	Alias                    string              `json:"alias"`
	DisplayName              string              `json:"displayName"`
	Source                   string              `json:"source"`
	Status                   string              `json:"status"`
	Message                  string              `json:"message,omitempty"`
	Email                    string              `json:"email,omitempty"`
	Plan                     string              `json:"plan,omitempty"`
	FiveHourLimit            string              `json:"fiveHourLimit,omitempty"`
	WeeklyLimit              string              `json:"weeklyLimit,omitempty"`
	PremiumRequestsUsed      int64               `json:"premiumRequestsUsed"`
	PremiumRequestsRemaining int64               `json:"premiumRequestsRemaining,omitempty"`
	PremiumRequestsMonth     string              `json:"premiumRequestsMonth,omitempty"`
	Balance                  deepSeekBalanceView `json:"balance"`
	Usage                    deepSeekUsageView   `json:"usage"`
	UsageUnavailable         bool                `json:"usageUnavailable"`
	UsageMessage             string              `json:"usageMessage,omitempty"`
	UpdatedAt                string              `json:"updatedAt,omitempty"`
}

type tokenScanPayload struct {
	OK        bool                      `json:"ok"`
	UpdatedAt string                    `json:"updatedAt"`
	Providers []tokenProviderScanResult `json:"providers"`
}

func (c *Client) fetchDeepSeekTokenStats(ctx context.Context, apiKey, rangeType, month string) (deepSeekTokenStatsPayload, error) {
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return deepSeekTokenStatsPayload{}, fmt.Errorf("apiKey is required")
	}

	normalizedRange := strings.ToLower(strings.TrimSpace(rangeType))
	if normalizedRange != "month" {
		normalizedRange = "day"
	}
	now := time.Now().UTC()
	normalizedMonth, startDate, endDate := normalizeStatsMonth(month, now)

	balance, err := c.fetchDeepSeekBalance(ctx, key)
	if err != nil {
		return deepSeekTokenStatsPayload{}, err
	}

	dayRows, usageUnavailable, usageMessage, err := c.fetchDeepSeekUsageRows(ctx, key, normalizedMonth, startDate, endDate)
	if err != nil {
		return deepSeekTokenStatsPayload{}, err
	}

	rows := dayRows
	if normalizedRange == "month" {
		rows = aggregateUsageRowsByMonth(dayRows)
	}

	return deepSeekTokenStatsPayload{
		OK:        true,
		Provider:  "deepseek",
		RangeType: normalizedRange,
		Month:     normalizedMonth,
		UpdatedAt: now.Format(time.RFC3339),
		Balance:   balance,
		Usage: deepSeekUsageView{
			RangeType: normalizedRange,
			Month:     normalizedMonth,
			Rows:      rows,
		},
		UsageUnavailable: usageUnavailable,
		UsageMessage:     usageMessage,
	}, nil
}

func (c *Client) fetchDeepSeekBalance(ctx context.Context, apiKey string) (deepSeekBalanceView, error) {
	base := strings.TrimRight(strings.TrimSpace(c.deepSeekBaseURL), "/")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	endpoint := base + "/user/balance"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return deepSeekBalanceView{}, fmt.Errorf("build deepseek balance request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return deepSeekBalanceView{}, fmt.Errorf("query deepseek balance: %w", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return deepSeekBalanceView{}, fmt.Errorf("deepseek api key is invalid or unauthorized")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = res.Status
		}
		return deepSeekBalanceView{}, fmt.Errorf("deepseek balance request failed: %s", message)
	}

	var parsed deepSeekBalanceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return deepSeekBalanceView{}, fmt.Errorf("decode deepseek balance response: %w", err)
	}

	items := make([]deepSeekBalanceInfo, 0, len(parsed.BalanceInfos))
	for _, info := range parsed.BalanceInfos {
		items = append(items, deepSeekBalanceInfo{
			Currency:        strings.TrimSpace(info.Currency),
			TotalBalance:    strings.TrimSpace(info.TotalBalance),
			GrantedBalance:  strings.TrimSpace(info.GrantedBalance),
			ToppedUpBalance: strings.TrimSpace(info.ToppedUpBalance),
		})
	}
	return deepSeekBalanceView{IsAvailable: parsed.IsAvailable, Items: items}, nil
}

func (c *Client) fetchDeepSeekUsageRows(ctx context.Context, apiKey, month string, startDate, endDate time.Time) ([]deepSeekUsageRow, bool, string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.deepSeekBaseURL), "/")
	if base == "" {
		base = "https://api.deepseek.com"
	}

	queries := []url.Values{
		{"granularity": []string{"day"}, "month": []string{month}},
		{"month": []string{month}},
		{"start_date": []string{startDate.Format("2006-01-02")}, "end_date": []string{endDate.Format("2006-01-02")}, "granularity": []string{"day"}},
	}
	endpoints := []string{"/user/usage", "/billing/usage", "/dashboard/billing/usage"}

	var lastMessage string
	for _, path := range endpoints {
		for _, query := range queries {
			u := base + path
			if encoded := query.Encode(); encoded != "" {
				u += "?" + encoded
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)

			res, err := c.httpClient.Do(req)
			if err != nil {
				lastMessage = err.Error()
				continue
			}
			body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
			res.Body.Close()

			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
				return nil, false, "", fmt.Errorf("deepseek api key is invalid or unauthorized")
			}
			if res.StatusCode == http.StatusNotFound {
				lastMessage = "usage endpoint not found"
				continue
			}
			if res.StatusCode < 200 || res.StatusCode >= 300 {
				if txt := strings.TrimSpace(string(body)); txt != "" {
					lastMessage = txt
				} else {
					lastMessage = res.Status
				}
				continue
			}

			rows := parseDeepSeekUsageRows(body)
			if len(rows) == 0 {
				lastMessage = "usage endpoint returned no parsable rows"
				continue
			}
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].Bucket < rows[j].Bucket
			})
			return rows, false, "", nil
		}
	}

	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "DeepSeek platform did not expose a usage API endpoint"
	}
	return []deepSeekUsageRow{}, true, lastMessage, nil
}

func normalizeStatsMonth(raw string, now time.Time) (string, time.Time, time.Time) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = now.Format("2006-01")
	}
	monthTime, err := time.Parse("2006-01", raw)
	if err != nil {
		monthTime = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	start := time.Date(monthTime.Year(), monthTime.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	return start.Format("2006-01"), start, end
}

func aggregateUsageRowsByMonth(rows []deepSeekUsageRow) []deepSeekUsageRow {
	if len(rows) == 0 {
		return []deepSeekUsageRow{}
	}
	agg := map[string]deepSeekUsageRow{}
	for _, row := range rows {
		bucket := strings.TrimSpace(row.Bucket)
		if len(bucket) >= 7 {
			bucket = bucket[:7]
		}
		if bucket == "" {
			continue
		}
		next := agg[bucket]
		next.Bucket = bucket
		next.InputTokens += row.InputTokens
		next.OutputTokens += row.OutputTokens
		next.TotalTokens += row.TotalTokens
		next.Cost += row.Cost
		agg[bucket] = next
	}
	out := make([]deepSeekUsageRow, 0, len(agg))
	for _, row := range agg {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Bucket < out[j].Bucket
	})
	return out
}

func parseDeepSeekUsageRows(body []byte) []deepSeekUsageRow {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil
	}
	rows := collectUsageRowsFromAny(doc)
	if len(rows) == 0 {
		return nil
	}
	return rows
}

func collectUsageRowsFromAny(v any) []deepSeekUsageRow {
	switch typed := v.(type) {
	case map[string]any:
		if row, ok := usageRowFromMap(typed); ok {
			return []deepSeekUsageRow{row}
		}
		keys := []string{"rows", "items", "data", "usage", "list", "result"}
		for _, key := range keys {
			if child, ok := typed[key]; ok {
				rows := collectUsageRowsFromAny(child)
				if len(rows) > 0 {
					return rows
				}
			}
		}
		for _, child := range typed {
			rows := collectUsageRowsFromAny(child)
			if len(rows) > 0 {
				return rows
			}
		}
	case []any:
		rows := make([]deepSeekUsageRow, 0, len(typed))
		for _, item := range typed {
			if m, ok := item.(map[string]any); ok {
				if row, ok := usageRowFromMap(m); ok {
					rows = append(rows, row)
				}
			}
		}
		if len(rows) > 0 {
			return rows
		}
	}
	return nil
}

func usageRowFromMap(m map[string]any) (deepSeekUsageRow, bool) {
	bucket := firstStringField(m, "date", "day", "stat_date", "month", "period", "time")
	if bucket == "" {
		return deepSeekUsageRow{}, false
	}
	inputTokens := firstInt64Field(m, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens")
	outputTokens := firstInt64Field(m, "output_tokens", "outputTokens", "completion_tokens", "completionTokens")
	totalTokens := firstInt64Field(m, "total_tokens", "totalTokens", "tokens")
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens
	}
	if totalTokens <= 0 {
		return deepSeekUsageRow{}, false
	}
	cost := firstFloat64Field(m, "cost", "amount", "usd", "cny")
	return deepSeekUsageRow{
		Bucket:       strings.TrimSpace(bucket),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		Cost:         cost,
	}, true
}

func firstStringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			switch typed := value.(type) {
			case string:
				trimmed := strings.TrimSpace(typed)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func firstInt64Field(m map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			switch typed := value.(type) {
			case float64:
				if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
					return int64(typed)
				}
			case json.Number:
				if n, err := typed.Int64(); err == nil {
					return n
				}
			case string:
				if n, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func firstFloat64Field(m map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			switch typed := value.(type) {
			case float64:
				if !math.IsNaN(typed) && !math.IsInf(typed, 0) {
					return typed
				}
			case json.Number:
				if n, err := typed.Float64(); err == nil {
					return n
				}
			case string:
				if n, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

type deepSeekCredential struct {
	Alias  string
	APIKey string
	Source string
}

type codexAuthProfile struct {
	Alias  string
	Source string
	Auth   map[string]any
}

type codexAuthState struct {
	AccessToken  string
	RefreshToken string
	AccountID    string
	Email        string
	Plan         string
}

type copilotProfile struct {
	Alias       string
	DisplayName string
	Source      string
}

type copilotPremiumUsageSummary struct {
	Used      int64
	Remaining int64
	Month     string
	HasQuota  bool
}

func (c *Client) scanTokenStats(ctx context.Context) (tokenScanPayload, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	deepSeek := c.scanDeepSeekProvider(ctx, now)
	codex := c.scanCodexProvider(ctx, now)
	copilot := c.scanCopilotProvider(ctx, now)
	return tokenScanPayload{
		OK:        true,
		UpdatedAt: now,
		Providers: []tokenProviderScanResult{deepSeek, codex, copilot},
	}, nil
}

func (c *Client) scanDeepSeekProvider(ctx context.Context, updatedAt string) tokenProviderScanResult {
	credentials := discoverDeepSeekCredentials()
	accounts := make([]tokenProviderAccount, 0, len(credentials))
	for _, credential := range credentials {
		alias := strings.TrimSpace(credential.Alias)
		if alias == "" {
			alias = "deepseek"
		}
		masked := maskSecret(credential.APIKey)
		account := tokenProviderAccount{
			ID:          alias + ":" + masked,
			Alias:       alias,
			DisplayName: alias,
			Source:      credential.Source,
			Status:      "ok",
			UpdatedAt:   updatedAt,
			Usage: deepSeekUsageView{
				RangeType: "month",
				Month:     time.Now().UTC().Format("2006-01"),
				Rows:      []deepSeekUsageRow{},
			},
		}
		stats, err := c.fetchDeepSeekTokenStats(ctx, credential.APIKey, "month", "")
		if err != nil {
			account.Status = "error"
			account.Message = err.Error()
			accounts = append(accounts, account)
			continue
		}
		account.Balance = stats.Balance
		account.Usage = stats.Usage
		account.UsageUnavailable = stats.UsageUnavailable
		account.UsageMessage = stats.UsageMessage
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return strings.ToLower(accounts[i].Alias) < strings.ToLower(accounts[j].Alias)
	})
	return tokenProviderScanResult{ID: "deepseek", Name: "DeepSeek", Accounts: accounts}
}

func (c *Client) scanCodexProvider(ctx context.Context, updatedAt string) tokenProviderScanResult {
	profiles := discoverCodexAuthProfiles()
	accounts := make([]tokenProviderAccount, 0, len(profiles))
	for _, profile := range profiles {
		state := extractCodexAuthState(profile.Auth)
		alias := strings.TrimSpace(profile.Alias)
		if alias == "" {
			alias = "codex"
		}
		accountID := strings.TrimSpace(state.AccountID)
		if accountID == "" {
			accountID = alias
		}
		account := tokenProviderAccount{
			ID:          accountID + ":" + alias,
			Alias:       alias,
			DisplayName: alias,
			Source:      profile.Source,
			Status:      "ok",
			Email:       state.Email,
			Plan:        state.Plan,
			UpdatedAt:   updatedAt,
		}
		fiveHour, weekly, plan, err := c.fetchCodexUsageLimits(ctx, state)
		if err != nil {
			account.Status = "error"
			account.Message = err.Error()
			accounts = append(accounts, account)
			continue
		}
		if strings.TrimSpace(plan) != "" {
			account.Plan = plan
		}
		account.FiveHourLimit = fiveHour
		account.WeeklyLimit = weekly
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return strings.ToLower(accounts[i].Alias) < strings.ToLower(accounts[j].Alias)
	})
	return tokenProviderScanResult{ID: "codex", Name: "Codex", Accounts: accounts}
}

func (c *Client) scanCopilotProvider(ctx context.Context, updatedAt string) tokenProviderScanResult {
	profile := discoverCopilotProfile()
	month := time.Now().UTC().Format("2006-01")
	account := tokenProviderAccount{
		ID:          "copilot:" + profile.Alias,
		Alias:       profile.Alias,
		DisplayName: profile.DisplayName,
		Source:      profile.Source,
		Status:      "ok",
		UpdatedAt:   updatedAt,
		Usage: deepSeekUsageView{
			RangeType: "month",
			Month:     month,
			Rows:      []deepSeekUsageRow{},
		},
	}
	token, tokenSource := discoverGitHubToken()
	if strings.TrimSpace(token) == "" {
		account.Status = "error"
		account.Message = "GitHub token not found for Copilot premium usage API (set COPILOT_GITHUB_TOKEN, GH_TOKEN, or GITHUB_TOKEN)"
		return tokenProviderScanResult{ID: "copilot", Name: "Copilot", Accounts: []tokenProviderAccount{account}}
	}
	if strings.TrimSpace(tokenSource) != "" {
		account.Source = account.Source + ", " + tokenSource
	}
	login, err := c.fetchGitHubLogin(ctx, token)
	if err != nil {
		account.Status = "error"
		account.Message = err.Error()
		return tokenProviderScanResult{ID: "copilot", Name: "Copilot", Accounts: []tokenProviderAccount{account}}
	}
	if strings.TrimSpace(login) != "" {
		account.Alias = login
		account.DisplayName = login
		account.Email = login
		account.ID = "copilot:" + login
	}
	summary, err := c.fetchCopilotPremiumUsage(ctx, token, login, time.Now().UTC())
	if err != nil {
		account.Status = "error"
		account.Message = err.Error()
		return tokenProviderScanResult{ID: "copilot", Name: "Copilot", Accounts: []tokenProviderAccount{account}}
	}
	account.PremiumRequestsUsed = summary.Used
	account.PremiumRequestsMonth = firstNonEmptyString(summary.Month, month)
	if summary.HasQuota {
		account.PremiumRequestsRemaining = summary.Remaining
	} else {
		account.UsageUnavailable = true
		account.UsageMessage = "GitHub API returned usage but no monthly quota; remaining premium requests unavailable"
	}
	return tokenProviderScanResult{ID: "copilot", Name: "Copilot", Accounts: []tokenProviderAccount{account}}
}
func discoverDeepSeekCredentials() []deepSeekCredential {
	out := make([]deepSeekCredential, 0)
	seen := map[string]struct{}{}
	appendCredential := func(alias, key, source string) {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			return
		}
		if _, ok := seen[trimmed]; ok {
			return
		}
		seen[trimmed] = struct{}{}
		out = append(out, deepSeekCredential{Alias: strings.TrimSpace(alias), APIKey: trimmed, Source: strings.TrimSpace(source)})
	}
	if value, ok := os.LookupEnv("DEEPSEEK_API_KEY"); ok {
		appendCredential("env", value, "env:DEEPSEEK_API_KEY")
	}
	if value, ok := os.LookupEnv("DEEPSEEK_API_KEYS"); ok {
		for _, item := range splitSecrets(value) {
			appendCredential("env", item, "env:DEEPSEEK_API_KEYS")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return out
	}
	fileSources := []struct {
		path   string
		source string
	}{
		{path: filepath.Join(home, ".config", "deepseek-cli", "config.json"), source: "~/.config/deepseek-cli/config.json"},
		{path: filepath.Join(home, ".deepseek-cli.json"), source: "~/.deepseek-cli.json"},
		{path: filepath.Join(home, ".wheelmaker", "deepseek-accounts.json"), source: "~/.wheelmaker/deepseek-accounts.json"},
		{path: filepath.Join(home, ".wheelmaker", "deepseek-api-keys.txt"), source: "~/.wheelmaker/deepseek-api-keys.txt"},
	}
	for _, fileSource := range fileSources {
		content, readErr := os.ReadFile(fileSource.path)
		if readErr != nil {
			continue
		}
		trimmed := strings.TrimSpace(string(content))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var doc any
			if json.Unmarshal(content, &doc) != nil {
				continue
			}
			for _, item := range collectDeepSeekKeysFromAny(doc, "") {
				alias := strings.TrimSpace(item.Alias)
				if alias == "" {
					alias = "file"
				}
				appendCredential(alias, item.APIKey, fileSource.source)
			}
			continue
		}
		for _, item := range strings.Split(trimmed, "\n") {
			appendCredential("file", item, fileSource.source)
		}
	}
	return out
}

func collectDeepSeekKeysFromAny(value any, fallbackAlias string) []deepSeekCredential {
	credentials := make([]deepSeekCredential, 0)
	switch typed := value.(type) {
	case map[string]any:
		alias := firstNonEmptyString(
			firstStringField(typed, "alias", "name", "id", "label"),
			fallbackAlias,
		)
		for _, keyName := range []string{"apiKey", "api_key", "deepseekApiKey", "deepseek_api_key", "token"} {
			if item, ok := typed[keyName]; ok {
				if secret, ok := item.(string); ok {
					credentials = append(credentials, deepSeekCredential{Alias: alias, APIKey: strings.TrimSpace(secret)})
				}
			}
		}
		for _, child := range typed {
			credentials = append(credentials, collectDeepSeekKeysFromAny(child, alias)...)
		}
	case []any:
		for _, child := range typed {
			credentials = append(credentials, collectDeepSeekKeysFromAny(child, fallbackAlias)...)
		}
	}
	return credentials
}

func splitSecrets(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", "\n")
	parts := strings.Split(normalized, "\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func discoverCodexAuthProfiles() []codexAuthProfile {
	profiles := make([]codexAuthProfile, 0)
	seen := map[string]struct{}{}
	appendProfile := func(alias, source string, auth map[string]any) {
		if len(auth) == 0 {
			return
		}
		state := extractCodexAuthState(auth)
		identity := firstNonEmptyString(state.AccountID, state.Email, state.AccessToken)
		if strings.TrimSpace(identity) == "" {
			return
		}
		if _, exists := seen[identity]; exists {
			return
		}
		seen[identity] = struct{}{}
		profiles = append(profiles, codexAuthProfile{Alias: strings.TrimSpace(alias), Source: strings.TrimSpace(source), Auth: auth})
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return profiles
	}
	codexDir := filepath.Join(home, ".codex")
	if auth := readJSONMapFile(filepath.Join(codexDir, "auth.json")); len(auth) > 0 {
		appendProfile("current", "~/.codex/auth.json", auth)
	}
	store := readJSONMapFile(filepath.Join(codexDir, "codex-cc.json"))
	if len(store) == 0 {
		return profiles
	}
	profilesNode, ok := store["profiles"].(map[string]any)
	if !ok {
		return profiles
	}
	for alias, rawProfile := range profilesNode {
		profileMap, ok := rawProfile.(map[string]any)
		if !ok {
			continue
		}
		provider := strings.ToLower(strings.TrimSpace(firstStringField(profileMap, "provider")))
		if provider != "" && provider != "codex" {
			continue
		}
		authNode, ok := profileMap["auth"].(map[string]any)
		if !ok {
			authNode, _ = profileMap["data"].(map[string]any)
		}
		appendProfile(alias, "~/.codex/codex-cc.json", authNode)
	}
	return profiles
}

func discoverCopilotProfile() copilotProfile {
	profile := copilotProfile{
		Alias:       "current",
		DisplayName: "Current Account",
		Source:      "~/.copilot/config.json",
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return profile
	}
	config := readJSONMapFile(filepath.Join(home, ".copilot", "config.json"))
	if len(config) == 0 {
		return profile
	}
	lastUser, _ := config["lastLoggedInUser"].(map[string]any)
	login := strings.TrimSpace(firstStringField(lastUser, "login"))
	host := strings.TrimSpace(firstStringField(lastUser, "host"))
	if login != "" {
		profile.Alias = login
		profile.DisplayName = login
	}
	if host != "" && profile.DisplayName != "" {
		profile.DisplayName = profile.DisplayName + "@" + host
	}
	return profile
}

func discoverGitHubToken() (string, string) {
	for _, key := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		if value, ok := os.LookupEnv(key); ok {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed, "env:" + key
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	copilotTokenFiles := []struct {
		path   string
		source string
	}{
		{path: filepath.Join(home, ".copilot", "apps.json"), source: "~/.copilot/apps.json"},
		{path: filepath.Join(home, ".copilot", "config.json"), source: "~/.copilot/config.json"},
		{path: filepath.Join(home, ".config", "github-copilot", "apps.json"), source: "~/.config/github-copilot/apps.json"},
		{path: filepath.Join(home, ".config", "github-copilot", "hosts.json"), source: "~/.config/github-copilot/hosts.json"},
		{path: filepath.Join(home, "AppData", "Local", "github-copilot", "apps.json"), source: "~/AppData/Local/github-copilot/apps.json"},
		{path: filepath.Join(home, "AppData", "Local", "github-copilot", "hosts.json"), source: "~/AppData/Local/github-copilot/hosts.json"},
	}
	for _, file := range copilotTokenFiles {
		token := extractGitHubTokenFromJSONFile(file.path)
		if token != "" {
			return token, file.source
		}
	}
	ghHostFiles := []struct {
		path   string
		source string
	}{
		{path: filepath.Join(home, ".config", "gh", "hosts.yml"), source: "~/.config/gh/hosts.yml"},
		{path: filepath.Join(home, ".config", "gh", "hosts.yaml"), source: "~/.config/gh/hosts.yaml"},
		{path: filepath.Join(home, "AppData", "Roaming", "GitHub CLI", "hosts.yml"), source: "~/AppData/Roaming/GitHub CLI/hosts.yml"},
		{path: filepath.Join(home, "AppData", "Roaming", "GitHub CLI", "hosts.yaml"), source: "~/AppData/Roaming/GitHub CLI/hosts.yaml"},
	}
	for _, file := range ghHostFiles {
		body, readErr := os.ReadFile(file.path)
		if readErr != nil {
			continue
		}
		if token := parseGitHubTokenFromHostsYAML(string(body)); token != "" {
			return token, file.source
		}
	}
	if token := discoverGitHubTokenByCLI(); token != "" {
		return token, "gh auth token"
	}
	return "", ""
}

func parseGitHubTokenFromHostsYAML(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r", "\n"), "\n")
	currentHost := ""
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(raw, " ") && strings.HasSuffix(trimmed, ":") {
			host := strings.TrimSuffix(trimmed, ":")
			host = strings.Trim(host, "\"'")
			currentHost = strings.ToLower(strings.TrimSpace(host))
			continue
		}
		if currentHost != "github.com" {
			continue
		}
		if !strings.HasPrefix(trimmed, "oauth_token:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "oauth_token:"))
		value = strings.Trim(value, "\"'")
		if value != "" {
			return value
		}
	}
	return ""
}

func extractGitHubTokenFromJSONFile(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return ""
	}
	var payload any
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	tokens := collectGitHubTokensFromAny(payload)
	for _, token := range tokens {
		if token != "" {
			return token
		}
	}
	return ""
}

func collectGitHubTokensFromAny(value any) []string {
	out := make([]string, 0)
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"oauth_token", "oauthToken", "token", "github_token", "githubToken", "access_token", "accessToken"} {
			if raw, ok := typed[key]; ok {
				if text, ok := raw.(string); ok {
					trimmed := strings.TrimSpace(text)
					if trimmed != "" {
						out = append(out, trimmed)
					}
				}
			}
		}
		for _, child := range typed {
			out = append(out, collectGitHubTokensFromAny(child)...)
		}
	case []any:
		for _, child := range typed {
			out = append(out, collectGitHubTokensFromAny(child)...)
		}
	}
	return out
}

func discoverGitHubTokenByCLI() string {
	cmd := exec.Command("gh", "auth", "token")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
func (c *Client) fetchGitHubLogin(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", fmt.Errorf("build github user request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("query github user: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return "", fmt.Errorf("github user request failed: http %d %s", res.StatusCode, msg)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode github user response: %w", err)
	}
	login := strings.TrimSpace(firstStringField(out, "login"))
	if login == "" {
		return "", fmt.Errorf("github user response missing login")
	}
	return login, nil
}

func (c *Client) fetchCopilotPremiumUsage(ctx context.Context, token, login string, now time.Time) (copilotPremiumUsageSummary, error) {
	username := strings.TrimSpace(login)
	if username == "" {
		return copilotPremiumUsageSummary{}, fmt.Errorf("missing github login for premium usage query")
	}
	month := now.UTC().Format("2006-01")
	year := now.UTC().Year()
	monthNumber := int(now.UTC().Month())
	endpoint := fmt.Sprintf("https://api.github.com/users/%s/settings/billing/premium_request/usage", url.PathEscape(username))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return copilotPremiumUsageSummary{}, fmt.Errorf("build copilot premium usage request: %w", err)
	}
	query := req.URL.Query()
	query.Set("year", strconv.Itoa(year))
	query.Set("month", strconv.Itoa(monthNumber))
	query.Set("product", "copilot")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return copilotPremiumUsageSummary{}, fmt.Errorf("query copilot premium usage: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return copilotPremiumUsageSummary{}, fmt.Errorf("copilot premium usage request failed: http %d %s", res.StatusCode, msg)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return copilotPremiumUsageSummary{}, fmt.Errorf("decode copilot premium usage response: %w", err)
	}
	items, _ := payload["usageItems"].([]any)
	var used int64
	var quota int64
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		product := strings.ToLower(strings.TrimSpace(firstStringField(item, "product", "name", "sku")))
		if product != "" && !strings.Contains(product, "copilot") {
			continue
		}
		quantity := firstFloat64Field(item, "netQuantity", "grossQuantity", "quantity", "used", "usedQuantity")
		if quantity > 0 {
			used += int64(math.Round(quantity))
		}
		quotaValue := firstFloat64Field(item, "totalMonthlyQuota", "total_monthly_quota", "monthlyQuota", "monthly_quota", "quota")
		if quotaValue > 0 {
			candidate := int64(math.Round(quotaValue))
			if candidate > quota {
				quota = candidate
			}
		}
	}
	if used <= 0 {
		fallbackUsed := firstFloat64Field(payload, "used", "usedQuantity", "netQuantity", "grossQuantity", "quantity")
		if fallbackUsed > 0 {
			used = int64(math.Round(fallbackUsed))
		}
	}
	if quota <= 0 {
		fallbackQuota := firstFloat64Field(payload, "totalMonthlyQuota", "total_monthly_quota", "monthlyQuota", "monthly_quota", "quota")
		if fallbackQuota > 0 {
			quota = int64(math.Round(fallbackQuota))
		}
	}
	summary := copilotPremiumUsageSummary{
		Used:      used,
		Month:     month,
		HasQuota:  quota > 0,
		Remaining: 0,
	}
	if quota > 0 {
		remaining := quota - used
		if remaining < 0 {
			remaining = 0
		}
		summary.Remaining = remaining
	}
	if period, ok := payload["timePeriod"].(map[string]any); ok {
		y := firstInt64Field(period, "year")
		m := firstInt64Field(period, "month")
		if y > 0 && m > 0 && m <= 12 {
			summary.Month = fmt.Sprintf("%04d-%02d", y, m)
		}
	}
	return summary, nil
}
func readJSONMapFile(path string) map[string]any {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

func extractCodexAuthState(auth map[string]any) codexAuthState {
	tokens, _ := auth["tokens"].(map[string]any)
	accessToken := strings.TrimSpace(firstStringField(tokens, "access_token", "accessToken"))
	refreshToken := strings.TrimSpace(firstStringField(tokens, "refresh_token", "refreshToken"))
	accountID := strings.TrimSpace(firstStringField(tokens, "account_id", "accountId"))
	email := ""
	plan := ""
	claims := decodeJWTPayload(accessToken)
	if len(claims) == 0 {
		idToken := strings.TrimSpace(firstStringField(tokens, "id_token", "idToken"))
		claims = decodeJWTPayload(idToken)
	}
	if len(claims) > 0 {
		profileMap, _ := claims["https://api.openai.com/profile"].(map[string]any)
		email = strings.TrimSpace(firstStringField(profileMap, "email"))
		authInfo, _ := claims["https://api.openai.com/auth"].(map[string]any)
		if accountID == "" {
			accountID = strings.TrimSpace(firstStringField(authInfo, "chatgpt_account_id"))
		}
		plan = strings.TrimSpace(firstStringField(authInfo, "chatgpt_plan_type"))
	}
	return codexAuthState{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccountID:    accountID,
		Email:        email,
		Plan:         normalizePlanLabel(plan),
	}
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(decoded, &out); err != nil {
		return nil
	}
	return out
}

func (c *Client) fetchCodexUsageLimits(ctx context.Context, state codexAuthState) (string, string, string, error) {
	access := strings.TrimSpace(state.AccessToken)
	if access == "" {
		return "", "", state.Plan, fmt.Errorf("missing access token")
	}
	payload, err := c.fetchCodexUsagePayload(ctx, access, state.AccountID)
	if err != nil {
		if !strings.Contains(err.Error(), "http 401") || strings.TrimSpace(state.RefreshToken) == "" {
			return "", "", state.Plan, err
		}
		refreshedAccess, refreshedAccountID, refreshErr := c.refreshCodexAccessToken(ctx, state.RefreshToken)
		if refreshErr != nil {
			return "", "", state.Plan, refreshErr
		}
		if refreshedAccountID != "" {
			state.AccountID = refreshedAccountID
		}
		if refreshedAccess == "" {
			return "", "", state.Plan, fmt.Errorf("refresh succeeded but access token is empty")
		}
		payload, err = c.fetchCodexUsagePayload(ctx, refreshedAccess, state.AccountID)
		if err != nil {
			return "", "", state.Plan, err
		}
	}
	fiveHour, weekly := pickCodexLimits(payload)
	plan := strings.TrimSpace(state.Plan)
	if plan == "" {
		plan = normalizePlanLabel(firstStringField(payload, "plan_type", "planType"))
	}
	return fiveHour, weekly, plan, nil
}

func (c *Client) fetchCodexUsagePayload(ctx context.Context, accessToken, accountID string) (map[string]any, error) {
	endpoint := "https://chatgpt.com/backend-api/wham/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build codex usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("User-Agent", "codex-cli")
	if trimmed := strings.TrimSpace(accountID); trimmed != "" {
		req.Header.Set("ChatGPT-Account-Id", trimmed)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query codex usage: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = res.Status
		}
		return nil, fmt.Errorf("codex usage request failed: http %d %s", res.StatusCode, msg)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode codex usage response: %w", err)
	}
	return payload, nil
}

func (c *Client) refreshCodexAccessToken(ctx context.Context, refreshToken string) (string, string, error) {
	payload := map[string]string{
		"client_id":     "app_EMoamEEZ73f0CkXaXp7hrann",
		"grant_type":    "refresh_token",
		"refresh_token": strings.TrimSpace(refreshToken),
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", strings.NewReader(string(body)))
	if err != nil {
		return "", "", fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "codex-cli")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("refresh access token: %w", err)
	}
	defer res.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(responseBody))
		if msg == "" {
			msg = res.Status
		}
		return "", "", fmt.Errorf("refresh token request failed: http %d %s", res.StatusCode, msg)
	}
	var out map[string]any
	if err := json.Unmarshal(responseBody, &out); err != nil {
		return "", "", fmt.Errorf("decode refresh token response: %w", err)
	}
	accessToken := strings.TrimSpace(firstStringField(out, "access_token", "accessToken"))
	accountID := strings.TrimSpace(firstStringField(out, "account_id", "accountId"))
	if accountID == "" {
		claims := decodeJWTPayload(accessToken)
		authInfo, _ := claims["https://api.openai.com/auth"].(map[string]any)
		accountID = strings.TrimSpace(firstStringField(authInfo, "chatgpt_account_id"))
	}
	return accessToken, accountID, nil
}

func pickCodexLimits(payload map[string]any) (string, string) {
	rateLimit, _ := payload["rate_limit"].(map[string]any)
	primary, _ := rateLimit["primary_window"].(map[string]any)
	secondary, _ := rateLimit["secondary_window"].(map[string]any)
	weeklyWindow := secondary
	if additional, ok := payload["additional_rate_limits"].([]any); ok {
		for _, item := range additional {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			name := strings.ToLower(firstStringField(entry, "metered_feature", "limit_name"))
			if !strings.Contains(name, "weekly") {
				continue
			}
			rate, _ := entry["rate_limit"].(map[string]any)
			window, _ := rate["primary_window"].(map[string]any)
			if len(window) > 0 {
				weeklyWindow = window
			}
			break
		}
	}
	return formatCodexWindow(primary), formatCodexWindow(weeklyWindow)
}

func formatCodexWindow(window map[string]any) string {
	if len(window) == 0 {
		return ""
	}
	used := firstFloat64Field(window, "used_percent", "usedPercent")
	if used < 0 {
		return ""
	}
	remaining := int64(math.Round(100 - used))
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 100 {
		remaining = 100
	}
	resetAt := firstInt64Field(window, "reset_at", "resetAt")
	if resetAt <= 0 {
		resetAfter := firstInt64Field(window, "reset_after_seconds", "resetAfterSeconds")
		if resetAfter > 0 {
			resetAt = time.Now().UTC().Add(time.Duration(resetAfter) * time.Second).Unix()
		}
	}
	if resetAt <= 0 {
		return fmt.Sprintf("%d%%", remaining)
	}
	return fmt.Sprintf("%d%% (%s)", remaining, time.Unix(resetAt, 0).Local().Format("01-02 15:04"))
}

func normalizePlanLabel(plan string) string {
	value := strings.ToLower(strings.TrimSpace(plan))
	switch value {
	case "plus":
		return "Plus"
	case "pro":
		return "Pro"
	case "team":
		return "Team"
	case "business":
		return "Business"
	case "enterprise", "enterprise/edu":
		return "Enterprise"
	default:
		if value == "" {
			return ""
		}
		return strings.ToUpper(value)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func maskSecret(secret string) string {
	trimmed := strings.TrimSpace(secret)
	if len(trimmed) <= 8 {
		return "****"
	}
	return trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
}
