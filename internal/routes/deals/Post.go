package deals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pipedrive_api_service/internal/client"
	"pipedrive_api_service/internal/utils"
)

type DealCreateItem struct {
	Title        string                 `json:"title"`
	Value        *float64               `json:"value,omitempty"`
	Currency     string                 `json:"currency,omitempty"`
	UserID       *int                   `json:"user_id,omitempty"`
	PipelineID   *int                   `json:"pipeline_id,omitempty"`
	StageID      *int                   `json:"stage_id,omitempty"`
	OrgID        *int                   `json:"org_id,omitempty"`
	PersonID     *int                   `json:"person_id,omitempty"`
	VisibleTo    *int                   `json:"visible_to,omitempty"`
	Status       string                 `json:"status,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	c := client.NewPipedriveClient()

	bodyRaw, err := io.ReadAll(r.Body)
	if err != nil {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "failed to read request body",
			"detail":  err.Error(),
		}, nil)
		return
	}
	defer r.Body.Close()

	if len(bodyRaw) == 0 {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "request body is empty",
			"hint":    "provide at least one deal to create",
		}, nil)
		return
	}

	var items []DealCreateItem

	if err := json.Unmarshal(bodyRaw, &items); err != nil {
		var single DealCreateItem
		if err2 := json.Unmarshal(bodyRaw, &single); err2 == nil && strings.TrimSpace(single.Title) != "" {
			items = append(items, single)
		} else {
			utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
				"message": "invalid JSON format",
				"hint":    "body must be either a single object or an array of objects",
				"example_single": map[string]string{
					"title": "New Sales Opportunity",
				},
				"example_bulk": []map[string]string{
					{"title": "Deal A"},
					{"title": "Deal B"},
				},
			}, nil)
			return
		}
	}

	if len(items) == 0 {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "no valid deals found in body",
			"hint":    "check your JSON structure and required fields",
		}, nil)
		return
	}

	results := make(map[string]interface{}, len(items))
	success := 0

	for i, item := range items {
		indexKey := fmt.Sprintf("%d", i)

		if strings.TrimSpace(item.Title) == "" {
			results[indexKey] = map[string]interface{}{
				"error":  "field 'title' is required",
				"status": http.StatusBadRequest,
			}
			continue
		}

		payload := map[string]interface{}{
			"title": item.Title,
		}
		if item.Value != nil {
			payload["value"] = *item.Value
		}
		if item.Currency != "" {
			payload["currency"] = item.Currency
		}
		if item.UserID != nil {
			payload["user_id"] = *item.UserID
		}
		if item.PipelineID != nil {
			payload["pipeline_id"] = *item.PipelineID
		}
		if item.StageID != nil {
			payload["stage_id"] = *item.StageID
		}
		if item.OrgID != nil {
			payload["org_id"] = *item.OrgID
		}
		if item.PersonID != nil {
			payload["person_id"] = *item.PersonID
		}
		if item.VisibleTo != nil {
			payload["visible_to"] = *item.VisibleTo
		}
		if item.Status != "" {
			payload["status"] = item.Status
		}
		for k, v := range item.CustomFields {
			payload[k] = v
		}

		bodyBytes, _ := json.Marshal(payload)

		reqCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		resp, body, _, err := c.DoWithBody(reqCtx, utils.HTTPPost, "/deals", nil, bytes.NewReader(bodyBytes))
		cancel()

		if err != nil || resp == nil {
			results[indexKey] = map[string]interface{}{
				"error":  fmt.Sprintf("failed to reach upstream Pipedrive: %v", err),
				"status": http.StatusServiceUnavailable,
			}
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			results[indexKey] = map[string]interface{}{
				"error":  fmt.Sprintf("upstream returned %d", resp.StatusCode),
				"status": resp.StatusCode,
			}
			continue
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(body, &parsed); err != nil {
			results[indexKey] = map[string]interface{}{
				"error":  fmt.Sprintf("unable to parse upstream response: %v", err),
				"status": http.StatusInternalServerError,
			}
			continue
		}

		if data, ok := parsed["data"]; ok {
			results[indexKey] = data
			success++
		} else {
			results[indexKey] = map[string]interface{}{
				"warning": "upstream response missing 'data' field",
				"status":  resp.StatusCode,
			}
		}
	}

	finalStatus := "success"
	if success == 0 {
		finalStatus = "failure"
	} else if success < len(items) {
		finalStatus = "partial_failure"
	}

	meta := utils.NewMetaItem(
		start,
		r.Header.Get(utils.HeaderXRequestID),
		c.BaseURL()+"/deals (create bulk)",
		http.StatusCreated,
		nil,
	)

	utils.JSONOK(w, map[string]interface{}{
		"status":  finalStatus,
		"results": results,
	}, meta)
}
