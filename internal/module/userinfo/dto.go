package userinfo

type UserInfoDTO struct {
	UserID    uint64 `json:"user_id"`
	City      string `json:"city"`
	Introduce string `json:"introduce"`
	Fans      uint32 `json:"fans"`
	Followee  uint32 `json:"followee"`
	Gender    uint8  `json:"gender"`
	Birthday  string `json:"birthday"`
	Credits   uint32 `json:"credits"`
	Level     uint8  `json:"level"`
}
