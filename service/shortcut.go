package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func (s *Service) registerShortcutRoutes(rg *gin.RouterGroup) {
	rg.POST("/shortcut", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		shortcutCreate := &api.ShortcutCreate{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(shortcutCreate); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post shortcut request")
			return
		}

		shortcutCreate.CreatorID = userID
		shortcut, err := s.Store.CreateShortcut(ctx, shortcutCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create shortcut")
			return
		}
		if err := s.createShortcutCreateActivity(ctx, shortcut); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(shortcut))
	})

	rg.PATCH("/shortcut/:shortcutId", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		shortcutID, err := strconv.Atoi(ctx.Param("shortcutId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("shortcutId")))
			return
		}

		shortcutFind := &api.ShortcutFind{
			ID: &shortcutID,
		}
		shortcut, err := s.Store.FindShortcut(ctx, shortcutFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find shortcut")
			return
		}
		if shortcut.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		currentTs := time.Now().Unix()
		shortcutPatch := &api.ShortcutPatch{
			UpdatedTs: &currentTs,
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(shortcutPatch); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted patch shortcut request")
			return
		}

		shortcutPatch.ID = shortcutID
		shortcut, err = s.Store.PatchShortcut(ctx, shortcutPatch)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to patch shortcut")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(shortcut))
	})

	rg.GET("/shortcut", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusBadRequest, "Missing user id to find shortcut")
			return
		}

		shortcutFind := &api.ShortcutFind{
			CreatorID: &userID,
		}
		list, err := s.Store.FindShortcutList(ctx, shortcutFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch shortcut list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(list))
	})

	rg.GET("/shortcut/:shortcutId", func(ctx *gin.Context) {
		shortcutID, err := strconv.Atoi(ctx.Param("shortcutId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("shortcutId")))
			return
		}

		shortcutFind := &api.ShortcutFind{
			ID: &shortcutID,
		}
		shortcut, err := s.Store.FindShortcut(ctx, shortcutFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to fetch shortcut by ID %d", *shortcutFind.ID))
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(shortcut))
	})

	rg.DELETE("/shortcut/:shortcutId", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		shortcutID, err := strconv.Atoi(ctx.Param("shortcutId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("shortcutId")))
			return
		}

		shortcutFind := &api.ShortcutFind{
			ID: &shortcutID,
		}
		shortcut, err := s.Store.FindShortcut(ctx, shortcutFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find shortcut")
			return
		}
		if shortcut.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		shortcutDelete := &api.ShortcutDelete{
			ID: &shortcutID,
		}
		if err := s.Store.DeleteShortcut(ctx, shortcutDelete); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Shortcut ID not found: %d", shortcutID))
				return
			}
			ctx.String(http.StatusInternalServerError, "Failed to delete shortcut")
			return
		}
		ctx.JSON(http.StatusOK, true)
	})
}

func (s *Service) createShortcutCreateActivity(ctx *gin.Context, shortcut *api.Shortcut) error {
	payload := api.ActivityShortcutCreatePayload{
		Title:   shortcut.Title,
		Payload: shortcut.Payload,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: shortcut.CreatorID,
		Type:      api.ActivityShortcutCreate,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
