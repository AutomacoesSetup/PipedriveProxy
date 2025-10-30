package models

// OwnerInfo representa o objeto aninhado 'owner_id' que o Pipedrive retorna
type OwnerInfo struct {
	ID         int    `json:"id"`
	Value      int    `json:"value"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	ActiveFlag bool   `json:"active_flag"`
	HasPic     int    `json:"has_pic"`
	PicHash    string `json:"pic_hash"`
}

// Organization representa a estrutura de dados base de uma organização para o modo de LISTAGEM.
// Nota: No modo de lookup por ID com fields=all, o Go usa map[string]interface{} para capturar todos os campos customizados.
type Organization struct {
	ID         int       `json:"id"`
	CompanyID  int       `json:"company_id"`
	Name       string    `json:"name"`
	OwnerID    OwnerInfo `json:"owner_id"`
	ActiveFlag bool      `json:"active_flag"`
	// Outros campos padrão podem ser adicionados aqui conforme necessário para o modo esparso.
}

// OrganizationsResponse é a estrutura de envelope para a listagem (GET sem ID)
type OrganizationsResponse struct {
	Success        bool           `json:"success"`
	Data           []Organization `json:"data"` // Slice da struct esparsa Organization
	Error          interface{}    `json:"error"`
	AdditionalData struct {
		Pagination struct {
			MoreItemsInCollection bool `json:"more_items_in_collection"`
			NextStart             int  `json:"next_start"`
		} `json:"pagination"`
	} `json:"additional_data"`
}

func (r *OrganizationsResponse) GetDataSlice() interface{} {
	return r.Data
}

func (r *OrganizationsResponse) SetDataSlice(data interface{}) {
	if filteredData, ok := data.([]Organization); ok {
		r.Data = filteredData
	}
}

// OrganizationUpdateData representa o corpo de uma requisição PUT/PATCH para atualização em massa
type OrganizationUpdateData map[string]map[string]interface{}

// BulkUpdateResult representa o item de retorno de uma operação de atualização em massa
type BulkUpdateResult struct {
	Status  string                 `json:"status"`
	Results map[string]interface{} `json:"results"`
}
