package routes

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"pipedrive_api_service/internal/client"
	"pipedrive_api_service/internal/utils"
)

type UpstreamCallFunc func(ctx context.Context, c *client.PipedriveClient, query url.Values, dataContainer interface{}) (*utils.RateLimitInfo, int, error)

type ResponseDataEnvelope interface {
	GetDataSlice() interface{}
	SetDataSlice(interface{})
}

func HandlerWrapper(callFunc UpstreamCallFunc, dataEnvelope ResponseDataEnvelope, endpointPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodGet {
			utils.JSONError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		c := client.NewPipedriveClient()

		rate, upstreamStatus, err := callFunc(ctx, c, r.URL.Query(), dataEnvelope)

		meta := utils.NewMetaItem(
			start,
			r.Header.Get(utils.HeaderXRequestID),
			c.BaseURL()+endpointPath,
			upstreamStatus,
			rate,
		)

		if err != nil {
			meta.Status = http.StatusServiceUnavailable
			utils.JSONError(w, http.StatusServiceUnavailable, err.Error(), meta)
			return
		}

		if upstreamStatus != http.StatusOK {
			utils.JSONError(w, upstreamStatus, fmt.Sprintf("upstream returned non-200: status %d", upstreamStatus), meta)
			return
		}

		// 1. Filtragem de dados local (e.g., ?name=Setup)
		dataSlice := dataEnvelope.GetDataSlice()
		filtered, filterErr := utils.FilterSliceByQuery(dataSlice, r.URL.Query())
		if filterErr != nil {
			utils.JSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to apply local filter: %v", filterErr), meta)
			return
		}

		// 2. Filtragem de campos (Field Selection) (e.g., ?fields=id,name)
		// O resultado final (finalData) será uma slice de structs OU uma slice de maps.
		finalData, fieldFilterErr := utils.FilterFieldsByQuery(filtered, r.URL.Query())
		if fieldFilterErr != nil {
			utils.JSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to apply field filter: %v", fieldFilterErr), meta)
			return
		}

		// 3. Atualizar metadados de total_results
		val := reflect.ValueOf(finalData)
		totalResults := 0
		if val.Kind() == reflect.Slice {
			totalResults = val.Len()
		}

		meta.Extra = &utils.ExtraMeta{TotalResults: totalResults}

		// 4. Retornar dados
		utils.JSONOK(w, finalData, meta)
	}
}
