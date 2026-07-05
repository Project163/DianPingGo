package shop

type CreateShopReq struct {
	Name     string `json:"name" binding:"required"`
	TypeID   uint64 `json:"type_id" binding:"required"`
	Images   string `json:"images"`
	Area     string `json:"area" binding:"required"`
	Address  string `json:"address" binding:"required"`
	OpenTime string `json:"open_time" binding:"required"`
}

type QueryShopReq struct {
	ID uint64 `json:"id" binding:"required,min=1"`
}

type QueryShopResp struct {
	ID        uint64  `json:"id"`
	Name      string  `json:"name"`
	TypeID    uint64  `json:"type_id"`
	Images    string  `json:"images"`
	Area      string  `json:"area"`
	Address   string  `json:"address"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	AvgPrice  uint64  `json:"avg_price"`
	Sold      uint    `json:"sold"`
	Comments  uint    `json:"comments"`
	Score     uint    `json:"score"`
	OpenTime  string  `json:"open_time"`
	Distance  float64 `json:"distance,omitempty"`
}

type UpdateShopReq struct {
	Name     string `json:"name"`
	TypeID   uint64 `json:"type_id"`
	Images   string `json:"images"`
	Area     string `json:"area"`
	Address  string `json:"address"`
	OpenTime string `json:"open_time"`
}
