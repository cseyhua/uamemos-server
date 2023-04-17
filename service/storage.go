package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
)

func (s *Service) registerStorageRoutes(rg *gin.RouterGroup) {
	rg.POST("/storage", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		user, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		if user == nil || user.Role != api.Host {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		storageCreate := &api.StorageCreate{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(storageCreate); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post storage request")
			return
		}

		storage, err := s.Store.CreateStorage(ctx, storageCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create storage")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(storage))
	})

	rg.PATCH("/storage/:storageId", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		user, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		if user == nil || user.Role != api.Host {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		storageID, err := strconv.Atoi(ctx.Param("storageId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("storageId")))
			return
		}

		storagePatch := &api.StoragePatch{
			ID: storageID,
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(storagePatch); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted patch storage request")
			return
		}

		storage, err := s.Store.PatchStorage(ctx, storagePatch)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to patch storage")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(storage))
	})

	rg.GET("/storage", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		user, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		// We should only show storage list to host user.
		if user == nil || user.Role != api.Host {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		storageList, err := s.Store.FindStorageList(ctx, &api.StorageFind{})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find storage list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(storageList))
	})

	rg.DELETE("/storage/:storageId", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		user, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		if user == nil || user.Role != api.Host {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		storageID, err := strconv.Atoi(ctx.Param("storageId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("storageId")))
			return
		}

		systemSetting, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{Name: api.SystemSettingStorageServiceIDName})
		if err != nil && common.ErrorCode(err) != common.NotFound {
			ctx.String(http.StatusInternalServerError, "Failed to find storage")
			return
		}
		if systemSetting != nil {
			storageServiceID := api.DatabaseStorage
			err = json.Unmarshal([]byte(systemSetting.Value), &storageServiceID)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to unmarshal storage service id")
				return
			}
			if storageServiceID == storageID {
				ctx.String(http.StatusBadRequest, fmt.Sprintf("Storage service %d is using", storageID))
				return
			}
		}

		if err = s.Store.DeleteStorage(ctx, &api.StorageDelete{ID: storageID}); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Storage ID not found: %d", storageID))
				return
			}
			ctx.String(http.StatusInternalServerError, "Failed to delete storage")
			return
		}
		ctx.JSON(http.StatusOK, true)
	})
}
