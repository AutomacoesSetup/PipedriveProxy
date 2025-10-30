package models

type Pipeline struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	URLTitle string `json:"url_title"`
	OrderNr  int    `json:"order_nr"`
	Active   bool   `json:"active"`
}

type PipelinesResponse struct {
	Success bool        `json:"success"`
	Data    []Pipeline  `json:"data"`
	Error   interface{} `json:"error"`
}

func (r *PipelinesResponse) GetDataSlice() interface{} {
	return r.Data
}

func (r *PipelinesResponse) SetDataSlice(data interface{}) {
	if filteredData, ok := data.([]Pipeline); ok {
		r.Data = filteredData
	}
}
