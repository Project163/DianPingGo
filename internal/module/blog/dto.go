package blog

// CreateBlogReq 创建博客请求参数
type CreateBlogReq struct {
	ShopID  uint64 `json:"shopId" binding:"required"`
	Title   string `json:"title" binding:"required,max=255"`
	Images  string `json:"images" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ScrollResult 滚动分页结果
type ScrollResult struct {
	List    []Blog `json:"list"`
	MinTime int64  `json:"minTime"`
	Offset  int64  `json:"offset"`
}
