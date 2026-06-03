package validator

import (
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func InitValidator() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("zh_mobile", func(fl validator.FieldLevel) bool {
			phone := fl.Field().String()
			return ValidateMobile(phone)
		})
	}
}
