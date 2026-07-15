package shoptype

type CreateShopTypeReq struct {
	Name string `json:"name" binding:"required"`
	Icon string `json:"icon" binding:"required"`
	Sort uint   `json:"sort" binding:"required"`
}

type GetShopTypeResp struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	Sort uint   `json:"sort"`
}
