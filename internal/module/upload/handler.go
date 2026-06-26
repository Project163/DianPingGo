package upload

import (
	"dianping/pkg/errmsg"
	"dianping/pkg/response"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	srv *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		srv: service,
	}
}

// UploadImage 处理图片上传请求，接受文件表单数据，调用服务层进行上传，并返回结果
func (h *Handler) UploadImage(ctx *gin.Context) {
	fh, err := ctx.FormFile("file")
	if err != nil {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, err))
		return
	}

	f, err := fh.Open()
	if err != nil {
		response.Fail(ctx, err)
		return
	}
	defer f.Close()

	path, err := h.srv.UploadImage(f, fh.Filename, fh.Size)
	if err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, path)
}

// DeleteImage 处理图片删除请求，接受文件名作为参数，调用服务层进行删除，并返回结果
func (h *Handler) DeleteImage(ctx *gin.Context) {
	filename := ctx.Query("name")
	if filename == "" {
		response.Fail(ctx, errmsg.NewError(errmsg.ErrInvalidParam, nil))
		return
	}

	if err := h.srv.DeleteImage(filename); err != nil {
		response.Fail(ctx, err)
		return
	}

	response.OK(ctx, nil)
}
