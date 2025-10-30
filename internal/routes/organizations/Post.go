package organizations

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

type OrganizationCreateItem struct {
	Name         string                 `json:"name"`
	OwnerID      *int                   `json:"owner_id,omitempty"`
	VisibleTo    *int                   `json:"visible_to,omitempty"`
	Address      string                 `json:"address,omitempty"`
	Label        string                 `json:"label,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	c := client.NewPipedriveClient()

	// Leitura bruta do body
	bodyRaw, err := io.ReadAll(r.Body)
	if err != nil {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "failed to read request body",
			"detail":  err.Error(),
		}, nil)
		return
	}
	defer r.Body.Close()

	// Detecta vazio
	if len(bodyRaw) == 0 {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "request body is empty",
			"hint":    "provide at least one organization to create",
		}, nil)
		return
	}

	var items []OrganizationCreateItem

	// 1. Tenta decodificar como lista
	if err := json.Unmarshal(bodyRaw, &items); err != nil {
		// 2. Tenta como único
		var single OrganizationCreateItem
		if err2 := json.Unmarshal(bodyRaw, &single); err2 == nil && strings.TrimSpace(single.Name) != "" {
			items = append(items, single)
		} else {
			utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
				"message": "invalid JSON format",
				"hint":    "body must be either a single object or an array of objects",
				"example_single": map[string]string{
					"name": "Setup Tecnologia LTDA",
				},
				"example_bulk": []map[string]string{
					{"name": "Empresa A"},
					{"name": "Empresa B"},
				},
			}, nil)
			return
		}
	}

	if len(items) == 0 {
		utils.JSONError(w, http.StatusBadRequest, map[string]interface{}{
			"message": "no valid organizations found in body",
			"hint":    "check your JSON structure and required fields",
		}, nil)
		return
	}

	results := make(map[string]interface{}, len(items))
	success := 0

	for i, item := range items {
		indexKey := fmt.Sprintf("%d", i)

		if strings.TrimSpace(item.Name) == "" {
			results[indexKey] = map[string]interface{}{
				"error":  "field 'name' is required",
				"status": http.StatusBadRequest,
			}
			continue
		}

		payload := map[string]interface{}{
			"name": item.Name,
		}
		if item.OwnerID != nil {
			payload["owner_id"] = *item.OwnerID
		}
		if item.VisibleTo != nil {
			payload["visible_to"] = *item.VisibleTo
		}
		if item.Address != "" {
			payload["address"] = item.Address
		}
		if item.Label != "" {
			payload["label"] = item.Label
		}
		for k, v := range item.CustomFields {
			payload[k] = v
		}

		bodyBytes, _ := json.Marshal(payload)

		reqCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		resp, body, _, err := c.DoWithBody(reqCtx, utils.HTTPPost, "/organizations", nil, bytes.NewReader(bodyBytes))
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
		c.BaseURL()+"/organizations (create bulk)",
		http.StatusCreated,
		nil,
	)

	utils.JSONOK(w, map[string]interface{}{
		"status":  finalStatus,
		"results": results,
	}, meta)
}
