package organizations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"pipedrive_api_service/internal/client"
	"pipedrive_api_service/internal/utils"
)

// HandleDelete removes one or multiple organizations by ID
func HandleDelete(w http.ResponseWriter, r *http.Request) {
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
			"message":        "request body is empty",
			"hint":           "provide at least one organization ID to delete",
			"example_single": map[string]int{"id": 123},
			"example_bulk":   []int{123, 124, 125},
		}, nil)
		return
	}

	// Suporta: [1,2,3] ou { "id": 123 }
	var ids []int
	if err := json.Unmarshal(bodyRaw, &ids); err != nil {
		var single map[string]interface{}
		if err2 := json.Unmarshal(bodyRaw, &single); err2 == nil {
			if idVal, ok := single["id"]; ok {
				switch v := idVal.(type) {
				case float64:
					ids = append(ids, int(v))
				case int:
					ids = append(ids, v)
				case string:
					if idNum, err := strconv.Atoi(v); err == nil {
						ids = append(ids, idNum)
					}
				}
			}
		}
	}

	if len(ids) == 0 {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "no valid organization IDs found in body",
			"hint":    "body must contain a numeric 'id' or a list of IDs",
		}, nil)
		return
	}

	results := make(map[string]interface{}, len(ids))
	successCount := 0

	for _, id := range ids {
		idStr := strconv.Itoa(id)
		if id <= 0 {
			results[idStr] = map[string]interface{}{
				"error":  "invalid id",
				"status": http.StatusBadRequest,
			}
			continue
		}

		path := fmt.Sprintf("/organizations/%d", id)
		reqCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		resp, body, _, err := c.Do(reqCtx, utils.HTTPDelete, path, nil)
		cancel()

		if err != nil || resp == nil {
			results[idStr] = map[string]interface{}{
				"error":  fmt.Sprintf("failed to reach upstream Pipedrive: %v", err),
				"status": http.StatusServiceUnavailable,
			}
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			var parsed map[string]interface{}
			_ = json.Unmarshal(body, &parsed)
			results[idStr] = map[string]interface{}{
				"error":  fmt.Sprintf("upstream returned %d", resp.StatusCode),
				"status": resp.StatusCode,
				"detail": parsed["error"],
			}
			continue
		}

		results[idStr] = map[string]interface{}{
			"success": true,
			"deleted": id,
		}
		successCount++
	}

	finalStatus := "success"
	if successCount == 0 {
		finalStatus = "failure"
	} else if successCount < len(ids) {
		finalStatus = "partial_failure"
	}

	meta := utils.NewMetaItem(
		start,
		r.Header.Get(utils.HeaderXRequestID),
		c.BaseURL()+"/organizations (delete bulk)",
		http.StatusOK,
		nil,
	)

	summary := map[string]int{
		"requested": len(ids),
		"deleted":   successCount,
		"failed":    len(ids) - successCount,
	}

	utils.JSONOK(w, map[string]interface{}{
		"status":  finalStatus,
		"summary": summary,
		"results": results,
	}, meta)
}
