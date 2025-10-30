package models

// PersonInfo representa o objeto 'person_id' dentro de um deal
type PersonInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	ActiveFlag bool   `json:"active_flag"`
}

// OrganizationInfo representa o objeto 'org_id' dentro de um deal
type OrganizationInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Deal representa a estrutura base de um negócio (Deal) em modo de listagem.
// Em consultas detalhadas (fields=all), campos adicionais e customizados
// podem ser capturados dinamicamente via map[string]interface{}.
type Deal struct {
	ID           int              `json:"id"`
	Title        string           `json:"title"`
	Value        float64          `json:"value"`
	Currency     string           `json:"currency"`
	Status       string           `json:"status"`
	StageID      int              `json:"stage_id"`
	PipelineID   int              `json:"pipeline_id"`
	Organization OrganizationInfo `json:"org_id"`
	Person       PersonInfo       `json:"person_id"`
	Owner        OwnerInfo        `json:"owner_id"` // reutiliza definição existente
	AddTime      string           `json:"add_time"`
	UpdateTime   string           `json:"update_time"`
	ActiveFlag   bool             `json:"active_flag"`
}

// DealsResponse é o envelope retornado no GET /deals (modo de listagem)
type DealsResponse struct {
	Success        bool        `json:"success"`
	Data           []Deal      `json:"data"`
	Error          interface{} `json:"error"`
	AdditionalData struct {
		Pagination struct {
			MoreItemsInCollection bool `json:"more_items_in_collection"`
			NextStart             int  `json:"next_start"`
		} `json:"pagination"`
	} `json:"additional_data"`
}

func (r *DealsResponse) GetDataSlice() interface{} {
	return r.Data
}

func (r *DealsResponse) SetDataSlice(data interface{}) {
	if filteredData, ok := data.([]Deal); ok {
		r.Data = filteredData
	}
}

// DealUpdateData representa o corpo da requisição PUT/PATCH para atualização em massa
type DealUpdateData map[string]map[string]interface{}
