package shoptype

type CreateShopTypeReq struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Sort uint   `json:"sort"`
}

type GetShopTypeResp struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Sort uint   `json:"sort"`
}
