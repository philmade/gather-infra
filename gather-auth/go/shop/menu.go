package shop

const ItemsPerPage = 5

type MenuItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Available    bool   `json:"available"`
	BasePriceBCH string `json:"base_price_bch"`
}

type CategoryInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Href  string `json:"href"`
}
