package service

import (
	"uamemos/common"

	"github.com/gin-gonic/gin"
)

type response struct {
	Data any `json:"data"`
}

func composeResponse(data any) response {
	return response{Data: data}
}

func (service *Service) defaultAuthSkipper(ctx *gin.Context) bool {
	path := ctx.Request.URL.Path
	return common.HasPrefixes(path, "/api/auth")
}
