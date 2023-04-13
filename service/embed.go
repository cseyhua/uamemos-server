package service

import (
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func embedFrontend(g *gin.Engine) {
	g.Use(static.Serve("/", static.LocalFile("dist", true)))
	assetsGroup := g.Group("assets/")
	assetsGroup.Use(func(ctx *gin.Context) {
		ctx.Request.Response.Header.Set("cache-control", "max-age=31536000, immutable")
		ctx.Next()
	})
	assetsGroup.Use(static.Serve("", static.LocalFile("dist/assets", true)))
}
