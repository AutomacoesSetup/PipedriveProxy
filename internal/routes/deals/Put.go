package deals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pipedrive_api_service/internal/client"
	"pipedrive_api_service/internal/models"
	"pipedrive_api_service/internal/utils"
)

type updateType string

const (
	updateReplace updateType = "replace"
	updateAdd     updateType = "add"
	updateRemove  updateType = "remove"

	defaultTimeout = 8 * time.Second
)

type DealUpdateItem struct {
	Type    string                 `json:"type"`
	Verbose bool                   `json:"verbose,omitempty"`
	ID      int                    `json:"id"`
	Fields  map[string]interface{} `json:"fields,omitempty"`
}

// Campos bloqueados para edição direta
var readOnlyFields = map[string]bool{
	"creator_user_id":   true,
	"owner_id":          true,
	"stage_change_time": true,
	"add_time":          true,
	"update_time":       true,
	"status":            false, // permitido
}

func HandlePut(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var items []DealUpdateItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		utils.JSONError(w, http.StatusBadRequest, "invalid JSON body", nil)
		return
	}
	if len(items) == 0 {
		utils.JSONError(w, http.StatusBadRequest, "empty update list", nil)
		return
	}

	c := client.NewPipedriveClient()
	results := make(map[string]interface{}, len(items))
	success := 0

	for _, it := range items {
		idStr := strconv.Itoa(it.ID)

		if it.ID <= 0 {
			results[idStr] = map[string]interface{}{
				"error":  "invalid id",
				"status": http.StatusBadRequest,
			}
			continue
		}

		op := normalizeType(it.Type)
		switch op {
		case updateReplace:
			if len(it.Fields) == 0 {
				results[idStr] = map[string]interface{}{
					"error":  "fields required for replace",
					"status": http.StatusBadRequest,
				}
				continue
			}
			if validationErr := validateFields(it.Fields); validationErr != nil {
				results[idStr] = map[string]interface{}{
					"error":  validationErr.Error(),
					"status": http.StatusBadRequest,
				}
				continue
			}
			res, status, err := doReplace(r.Context(), c, it.ID, it.Fields, it.Verbose)
			if err != nil {
				results[idStr] = map[string]interface{}{
					"error":  err.Error(),
					"status": status,
				}
				continue
			}
			results[idStr] = res
			success++

		case updateAdd:
			if len(it.Fields) == 0 {
				results[idStr] = map[string]interface{}{
					"error":  "fields required for add",
					"status": http.StatusBadRequest,
				}
				continue
			}
			if validationErr := validateFields(it.Fields); validationErr != nil {
				results[idStr] = map[string]interface{}{
					"error":  validationErr.Error(),
					"status": http.StatusBadRequest,
				}
				continue
			}
			res, status, err := doAdd(r.Context(), c, it.ID, it.Fields, it.Verbose)
			if err != nil {
				results[idStr] = map[string]interface{}{
					"error":  err.Error(),
					"status": status,
				}
				continue
			}
			results[idStr] = res
			success++

		case updateRemove:
			if len(it.Fields) == 0 {
				results[idStr] = map[string]interface{}{
					"error":  "fields (keys) required for remove",
					"status": http.StatusBadRequest,
				}
				continue
			}
			res, status, err := doRemove(r.Context(), c, it.ID, it.Fields, it.Verbose)
			if err != nil {
				results[idStr] = map[string]interface{}{
					"error":  err.Error(),
					"status": status,
				}
				continue
			}
			results[idStr] = res
			success++

		default:
			results[idStr] = map[string]interface{}{
				"error":  "unsupported type",
				"status": http.StatusBadRequest,
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
		c.BaseURL()+"/deals (bulk)",
		http.StatusMultiStatus,
		nil,
	)

	resp := models.BulkUpdateResult{
		Status:  finalStatus,
		Results: results,
	}
	utils.JSONOK(w, resp, meta)
}

// --- Helpers ---

func doReplace(ctx context.Context, c *client.PipedriveClient, id int, fields map[string]interface{}, verbose bool) (interface{}, int, error) {
	bodyBytes, err := json.Marshal(fields)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("marshal fields: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	resp, body, _, err := c.DoWithBody(reqCtx, utils.HTTPPut, "/deals/"+strconv.Itoa(id), nil, bytes.NewReader(bodyBytes))
	if err != nil {
		status := http.StatusServiceUnavailable
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, status, wrapUpstreamErr("replace", err, body)
	}
	defer resp.Body.Close()

	var parsed map[string]interface{}
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode >= 400 {
		detail := extractUpstreamError(parsed)
		return parsed, resp.StatusCode, fmt.Errorf(detail)
	}

	if !verbose {
		return map[string]interface{}{
			"success":        true,
			"fields_altered": extractFieldKeys(fields),
		}, resp.StatusCode, nil
	}
	return parsed["data"], resp.StatusCode, nil
}

func doAdd(ctx context.Context, c *client.PipedriveClient, id int, additions map[string]interface{}, verbose bool) (interface{}, int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	current, status, err := fetchDealAsMap(reqCtx, c, id, nil)
	if err != nil {
		return nil, status, fmt.Errorf("fetch before add: %w", err)
	}

	update := make(map[string]interface{}, len(additions))
	for k, v := range additions {
		old := fmt.Sprint(current[k])
		newVal := strings.TrimSpace(strings.Join([]string{old, fmt.Sprint(v)}, " "))
		newVal = strings.TrimSpace(strings.ReplaceAll(newVal, "  ", " "))
		update[k] = newVal
	}

	return doReplace(ctx, c, id, update, verbose)
}

func doRemove(ctx context.Context, c *client.PipedriveClient, id int, fields map[string]interface{}, verbose bool) (interface{}, int, error) {
	clear := make(map[string]interface{}, len(fields))
	for k := range fields {
		clear[k] = ""
	}
	return doReplace(ctx, c, id, clear, verbose)
}

func fetchDealAsMap(ctx context.Context, c *client.PipedriveClient, id int, q url.Values) (map[string]interface{}, int, error) {
	if q == nil {
		q = url.Values{}
	}

	reqCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	resp, body, _, err := c.Do(reqCtx, utils.HTTPGet, "/deals/"+strconv.Itoa(id), q)
	if err != nil {
		status := http.StatusServiceUnavailable
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, status, wrapUpstreamErr("get", err, body)
	}

	if resp.StatusCode != http.StatusOK {
		var raw map[string]interface{}
		_ = json.Unmarshal(body, &raw)
		return raw, resp.StatusCode, fmt.Errorf("upstream status %s", resp.Status)
	}

	var parsed struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
		Error   interface{}            `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("parse upstream response: %w", err)
	}
	return parsed.Data, http.StatusOK, nil
}

func normalizeType(t string) updateType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "replace":
		return updateReplace
	case "add":
		return updateAdd
	case "remove":
		return updateRemove
	default:
		return updateType("unsupported")
	}
}

func wrapUpstreamErr(op string, err error, body []byte) error {
	if len(body) == 0 {
		return fmt.Errorf("%s: %w", op, err)
	}
	return fmt.Errorf("%s: %w; upstream=%s", op, err, string(body))
}

func extractUpstreamError(parsed map[string]interface{}) string {
	if errVal, ok := parsed["error"]; ok && errVal != nil {
		switch e := errVal.(type) {
		case string:
			return e
		case map[string]interface{}:
			if msg, ok := e["message"].(string); ok {
				return msg
			}
			if errList, ok := e["errors"].([]interface{}); ok && len(errList) > 0 {
				return fmt.Sprint(errList[0])
			}
			return fmt.Sprint(e)
		default:
			return fmt.Sprint(e)
		}
	}
	return "upstream returned an unspecified error"
}

func extractFieldKeys(fields map[string]interface{}) []string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	return keys
}

func validateFields(fields map[string]interface{}) error {
	for k := range fields {
		if readOnlyFields[k] {
			return fmt.Errorf("field '%s' is read-only and cannot be modified", k)
		}
	}
	for k, v := range fields {
		strVal := fmt.Sprint(v)
		if strings.EqualFold(k, "value") {
			if _, err := strconv.ParseFloat(strVal, 64); err != nil {
				return fmt.Errorf("invalid numeric value for 'value'")
			}
		}
		if strings.Contains(k, "id") && k != "owner_id" {
			if _, err := strconv.Atoi(strVal); err != nil {
				return fmt.Errorf("invalid numeric value for '%s'", k)
			}
		}
	}
	return nil
}
