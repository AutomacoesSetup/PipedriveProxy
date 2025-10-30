package utils

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type RateLimitInfo struct {
	Limit     int   `json:"limit,omitempty"`
	Remaining int   `json:"remaining,omitempty"`
	ResetAt   int64 `json:"reset_at,omitempty"`
}

type TokenUsage struct {
	Prompt     int `json:"prompt,omitempty"`
	Completion int `json:"completion,omitempty"`
	Total      int `json:"total,omitempty"`
}

type ExtraMeta struct {
	TotalResults int `json:"total_results,omitempty"`
}

type MetaItem struct {
	RequestID  string         `json:"request_id,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	URL        string         `json:"url,omitempty"`
	Status     int            `json:"status,omitempty"`
	RateLimit  *RateLimitInfo `json:"rate_limit,omitempty"`
	Tokens     *TokenUsage    `json:"tokens,omitempty"`
	Extra      *ExtraMeta     `json:"extra,omitempty"`
}

type Envelope struct {
	Success  bool        `json:"success"`
	Data     interface{} `json:"data,omitempty"`
	Error    interface{} `json:"error,omitempty"`
	Metadata []MetaItem  `json:"metadata,omitempty"`
}

func JSON(w http.ResponseWriter, status int, payload Envelope) {
	w.Header().Set(HeaderContentType, ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func JSONOK(w http.ResponseWriter, data interface{}, meta *MetaItem) {
	metadata := []MetaItem{}
	if meta != nil {
		metadata = append(metadata, *meta)
	}
	JSON(w, http.StatusOK, Envelope{
		Success:  true,
		Data:     data,
		Metadata: metadata,
	})
}

func JSONError(w http.ResponseWriter, status int, err interface{}, meta *MetaItem) {
	metadata := []MetaItem{}
	if meta != nil {
		metadata = append(metadata, *meta)
	}
	JSON(w, status, Envelope{
		Success:  false,
		Error:    err,
		Metadata: metadata,
	})
}

func NewMetaItem(started time.Time, requestID string, url string, status int, rateLimit *RateLimitInfo) *MetaItem {
	return &MetaItem{
		RequestID:  requestID,
		DurationMs: time.Since(started).Milliseconds(),
		URL:        url,
		Status:     status,
		RateLimit:  rateLimit,
	}
}

func ExtractRateLimitFromHeaders(h http.Header) *RateLimitInfo {
	limit, _ := strconv.Atoi(h.Get(HeaderXRateLimitLimit))
	remaining, _ := strconv.Atoi(h.Get(HeaderXRateLimitRemaining))

	var resetAt int64
	if h.Get(HeaderXRateLimitReset) != "" {
		if v, err := strconv.ParseInt(h.Get(HeaderXRateLimitReset), 10, 64); err == nil {
			resetAt = v
		}
	}

	if limit == 0 && remaining == 0 && resetAt == 0 {
		return nil
	}
	return &RateLimitInfo{Limit: limit, Remaining: remaining, ResetAt: resetAt}
}
