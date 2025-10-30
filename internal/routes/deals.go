package routes

import (
	"net/http"

	org "pipedrive_api_service/internal/routes/deals"
	"pipedrive_api_service/internal/utils"
)

func DealsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		org.HandleGet(w, r)
	case http.MethodPost:
		org.HandlePost(w, r)
	case http.MethodDelete:
		org.HandleDelete(w, r)
	case http.MethodPut:
		org.HandlePut(w, r)
	default:
		utils.JSONError(w, http.StatusMethodNotAllowed, "method not allowed", nil)
	}
}
