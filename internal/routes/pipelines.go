package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"pipedrive_api_service/internal/client"
	"pipedrive_api_service/internal/models"
	"pipedrive_api_service/internal/utils"
)

func pipelinesUpstreamCall(ctx context.Context, c *client.PipedriveClient, query url.Values, dataContainer interface{}) (*utils.RateLimitInfo, int, error) {

	resp, body, rate, err := c.Do(ctx, utils.HTTPGet, "/pipelines", query)

	if err != nil {
		return rate, http.StatusServiceUnavailable, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return rate, resp.StatusCode, fmt.Errorf("upstream returned status: %s", resp.Status)
	}

	pipedriveResponse := dataContainer.(*models.PipelinesResponse)

	if err := json.Unmarshal(body, pipedriveResponse); err != nil {
		return rate, http.StatusInternalServerError, fmt.Errorf("failed to parse upstream response: %w", err)
	}

	return rate, http.StatusOK, nil
}

func PipelinesHandler(w http.ResponseWriter, r *http.Request) {
	envelope := &models.PipelinesResponse{}
	HandlerWrapper(pipelinesUpstreamCall, envelope, "/pipelines")(w, r)
}
