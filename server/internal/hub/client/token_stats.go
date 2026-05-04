package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
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
