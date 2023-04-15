package service

import (
	"fmt"
	"net/http"

	"uamemos/api"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerUserRoutes(rg *gin.RouterGroup) {
	rg.GET("/user/me", func(ctx *gin.Context) {
		userID, ok := ctx.Get(getUserIDContextKey())
		fmt.Printf("\n%d\n", userID)
		if !ok {
			ctx.String(http.StatusUnauthorized, "Missing auth session")
			return
		}

		_userID, _ := userID.(int)

		userFind := &api.UserFind{
			ID: &_userID,
		}
		user, err := s.Store.FindUser(ctx, userFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}

		userSettingList, err := s.Store.FindUserSettingList(ctx, &api.UserSettingFind{
			UserID: _userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find userSettingList")
			return
		}
		user.UserSettingList = userSettingList
		ctx.JSON(http.StatusOK, composeResponse(user))
	})
}
