package organizations

import (
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

const OrganizationsPageLimit = 500

type FieldMeta struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	FieldType string `json:"field_type"`
}

var orgFieldCache = map[string]FieldMeta{}

func fetchOrganizationFields(ctx context.Context, c *client.PipedriveClient) (map[string]FieldMeta, error) {
	if len(orgFieldCache) > 0 {
		return orgFieldCache, nil
	}

	resp, body, _, err := c.Do(ctx, utils.HTTPGet, "/organizationFields", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch organization fields: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned %d while fetching organizationFields", resp.StatusCode)
	}

	var result struct {
		Success bool        `json:"success"`
		Data    []FieldMeta `json:"data"`
		Error   interface{} `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse organizationFields: %w", err)
	}

	for _, f := range result.Data {
		orgFieldCache[f.Key] = f
	}
	return orgFieldCache, nil
}

func fetchMultipleOrganizationDetails(ctx context.Context, c *client.PipedriveClient, ids []string, query url.Values) ([]map[string]interface{}, *utils.RateLimitInfo, int, error) {
	results := make([]map[string]interface{}, 0, len(ids))
	latestRate := &utils.RateLimitInfo{}
	overallStatus := http.StatusOK

	fieldsMeta, _ := fetchOrganizationFields(ctx, c)

	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			continue
		}

		path := fmt.Sprintf("/organizations/%s", id)
		currentQuery := make(url.Values)
		for k, v := range query {
			currentQuery[k] = v
		}

		detailCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, body, rate, err := c.Do(detailCtx, utils.HTTPGet, path, currentQuery)
		cancel()

		if rate != nil {
			latestRate = rate
		}
		if err != nil {
			if ctx.Err() != nil {
				return results, latestRate, http.StatusGatewayTimeout, ctx.Err()
			}
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var pipedriveResponse struct {
			Success bool                   `json:"success"`
			Data    map[string]interface{} `json:"data"`
			Error   interface{}            `json:"error"`
		}
		if err := json.Unmarshal(body, &pipedriveResponse); err != nil {
			continue
		}

		org := pipedriveResponse.Data
		customFields := make([]map[string]interface{}, 0)

		for key, value := range org {
			if meta, ok := fieldsMeta[key]; ok {
				customFields = append(customFields, map[string]interface{}{
					"id":    meta.Key,
					"name":  meta.Name,
					"type":  meta.FieldType,
					"value": value,
				})
				delete(org, key)
			}
		}

		if len(customFields) > 0 {
			org["custom_fields"] = customFields
		}

		results = append(results, org)
	}

	if len(results) == 0 && len(ids) > 0 {
		return results, latestRate, http.StatusNotFound, fmt.Errorf("no organizations found for the provided IDs")
	}
	return results, latestRate, overallStatus, nil
}

func listOrganizations(ctx context.Context, c *client.PipedriveClient, query url.Values, dataContainer interface{}) (*utils.RateLimitInfo, int, error) {
	finalResponse := dataContainer.(*models.OrganizationsResponse)
	finalResponse.Data = []models.Organization{}

	rateLimitInfo := &utils.RateLimitInfo{}
	upstreamStatus := http.StatusOK

	pageFilter := query.Get("page")
	query.Del("page")

	isPageAll := pageFilter == "all"
	start := 0

	if pageFilter != "" && pageFilter != "all" {
		page, err := strconv.Atoi(pageFilter)
		if err == nil && page > 0 {
			start = (page - 1) * OrganizationsPageLimit
		}
	}

	if query.Get("limit") == "" {
		query.Set("limit", fmt.Sprintf("%d", OrganizationsPageLimit))
	}

	for {
		if ctx.Err() != nil {
			return rateLimitInfo, http.StatusGatewayTimeout, fmt.Errorf("gateway process cancelled: %w", ctx.Err())
		}

		currentQuery := make(url.Values)
		for k, v := range query {
			currentQuery[k] = v
		}
		currentQuery.Set("start", fmt.Sprintf("%d", start))

		resp, body, rate, err := c.Do(ctx, utils.HTTPGet, "/organizations", currentQuery)
		if err != nil {
			return rate, http.StatusServiceUnavailable, err
		}

		if rate != nil {
			rateLimitInfo = rate
		}
		upstreamStatus = resp.StatusCode

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return rateLimitInfo, upstreamStatus, fmt.Errorf("upstream returned status: %s", resp.Status)
		}

		tempResponse := models.OrganizationsResponse{}
		if err := json.Unmarshal(body, &tempResponse); err != nil {
			resp.Body.Close()
			return rateLimitInfo, http.StatusInternalServerError, fmt.Errorf("failed to parse upstream response: %w", err)
		}
		resp.Body.Close()

		finalResponse.Data = append(finalResponse.Data, tempResponse.Data...)

		if !isPageAll || !tempResponse.AdditionalData.Pagination.MoreItemsInCollection {
			break
		}

		start = tempResponse.AdditionalData.Pagination.NextStart
	}

	return rateLimitInfo, upstreamStatus, nil
}

func HandleGet(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	id := query.Get("id")

	fieldsQuery := query.Get("fields")
	fieldsAll := strings.EqualFold(fieldsQuery, "all")
	query.Del("fields")

	if id != "" {
		ids := strings.Split(id, ",")
		query.Del("id")

		c := client.NewPipedriveClient()
		start := time.Now()

		dataToReturn, rate, upstreamStatus, err := fetchMultipleOrganizationDetails(r.Context(), c, ids, query)

		meta := utils.NewMetaItem(
			start,
			r.Header.Get(utils.HeaderXRequestID),
			c.BaseURL()+fmt.Sprintf("/organizations/details/{%d IDs}", len(ids)),
			upstreamStatus,
			rate,
		)

		if err != nil {
			meta.Status = upstreamStatus
			utils.JSONError(w, upstreamStatus, err.Error(), meta)
			return
		}

		if fieldsQuery != "" && !fieldsAll {
			var filterErr error
			dataToReturn, filterErr = utils.FilterMapSliceByFields(dataToReturn, fieldsQuery)
			if filterErr != nil {
				meta.Status = http.StatusInternalServerError
				utils.JSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to filter fields: %v", filterErr), meta)
				return
			}
		}

		meta.Extra = &utils.ExtraMeta{TotalResults: len(dataToReturn)}
		utils.JSONOK(w, dataToReturn, meta)
		return
	}

	c := client.NewPipedriveClient()
	start := time.Now()
	envelope := &models.OrganizationsResponse{}
	rate, upstreamStatus, err := listOrganizations(r.Context(), c, query, envelope)

	meta := utils.NewMetaItem(
		start,
		r.Header.Get(utils.HeaderXRequestID),
		c.BaseURL()+"/organizations",
		upstreamStatus,
		rate,
	)

	if err != nil {
		meta.Status = upstreamStatus
		utils.JSONError(w, upstreamStatus, err.Error(), meta)
		return
	}

	meta.Extra = &utils.ExtraMeta{TotalResults: len(envelope.Data)}
	utils.JSONOK(w, envelope, meta)
}
