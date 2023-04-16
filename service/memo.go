package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func (s *Service) registerMemoRoutes(rg *gin.RouterGroup) {
	rg.POST("/memo", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		memoCreate := &api.MemoCreate{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(memoCreate); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post memo request")
			return
		}

		if memoCreate.Visibility == "" {
			userMemoVisibilitySetting, err := s.Store.FindUserSetting(ctx, &api.UserSettingFind{
				UserID: userID,
				Key:    api.UserSettingMemoVisibilityKey,
			})
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to find user setting")
				return
			}

			if userMemoVisibilitySetting != nil {
				memoVisibility := api.Private
				err := json.Unmarshal([]byte(userMemoVisibilitySetting.Value), &memoVisibility)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to unmarshal user setting value")
					return
				}
				memoCreate.Visibility = memoVisibility
			} else {
				// Private is the default memo visibility.
				memoCreate.Visibility = api.Private
			}
		}

		// Find system settings
		disablePublicMemosSystemSetting, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{
			Name: api.SystemSettingDisablePublicMemosName,
		})
		if err != nil && common.ErrorCode(err) != common.NotFound {
			ctx.String(http.StatusInternalServerError, "Failed to find system setting")
			return
		}
		if disablePublicMemosSystemSetting != nil {
			disablePublicMemos := false
			err = json.Unmarshal([]byte(disablePublicMemosSystemSetting.Value), &disablePublicMemos)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to unmarshal system setting")
				return
			}
			if disablePublicMemos {
				// Allow if the user is an admin.
				user, err := s.Store.FindUser(ctx, &api.UserFind{
					ID: &userID,
				})
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to find user")
					return
				}
				// Only enforce private if you're a regular user.
				// Admins should know what they're doing.
				if user.Role == "USER" {
					memoCreate.Visibility = api.Private
				}
			}
		}

		if len(memoCreate.Content) > api.MaxContentLength {
			ctx.String(http.StatusBadRequest, "Content size overflow, up to 1MB")
			return
		}

		memoCreate.CreatorID = userID
		memo, err := s.Store.CreateMemo(ctx, memoCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create memo")
			return
		}
		if err := s.createMemoCreateActivity(ctx, memo); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}

		for _, resourceID := range memoCreate.ResourceIDList {
			if _, err := s.Store.UpsertMemoResource(ctx, &api.MemoResourceUpsert{
				MemoID:     memo.ID,
				ResourceID: resourceID,
			}); err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to upsert memo resource")
				return
			}
		}

		memo, err = s.Store.ComposeMemo(ctx, memo)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to compose memo")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(memo))
	})

	rg.PATCH("/memo/:memoId", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		memo, err := s.Store.FindMemo(ctx, &api.MemoFind{
			ID: &memoID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find memo")
			return
		}
		if memo.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		currentTs := time.Now().Unix()
		memoPatch := &api.MemoPatch{
			ID:        memoID,
			UpdatedTs: &currentTs,
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(memoPatch); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted patch memo request")
			return
		}

		if memoPatch.Content != nil && len(*memoPatch.Content) > api.MaxContentLength {
			ctx.String(http.StatusBadRequest, "Content size overflow, up to 1MB")
			return
		}

		memo, err = s.Store.PatchMemo(ctx, memoPatch)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to patch memo")
			return
		}

		for _, resourceID := range memoPatch.ResourceIDList {
			if _, err := s.Store.UpsertMemoResource(ctx, &api.MemoResourceUpsert{
				MemoID:     memo.ID,
				ResourceID: resourceID,
			}); err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to upsert memo resource")
				return
			}
		}

		memo, err = s.Store.ComposeMemo(ctx, memo)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to compose memo")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(memo))
	})

	rg.GET("/memo", func(ctx *gin.Context) {

		memoFind := &api.MemoFind{}
		if userID, err := strconv.Atoi(ctx.Query("creatorId")); err == nil {
			memoFind.CreatorID = &userID
		}

		_currentUserID, ok := ctx.Get(getUserIDContextKey())
		currentUserID, _ok := _currentUserID.(int)
		if !ok || !_ok {
			if memoFind.CreatorID == nil {
				ctx.String(http.StatusBadRequest, "Missing user id to find memo")
				return
			}
			memoFind.VisibilityList = []api.Visibility{api.Public}
		} else {
			if memoFind.CreatorID == nil {
				memoFind.CreatorID = &currentUserID
			} else {
				memoFind.VisibilityList = []api.Visibility{api.Public, api.Protected}
			}
		}

		rowStatus := api.RowStatus(ctx.Query("rowStatus"))
		if rowStatus != "" {
			memoFind.RowStatus = &rowStatus
		}
		pinnedStr := ctx.Query("pinned")
		if pinnedStr != "" {
			pinned := pinnedStr == "true"
			memoFind.Pinned = &pinned
		}
		tag := ctx.Query("tag")
		if tag != "" {
			contentSearch := "#" + tag
			memoFind.ContentSearch = &contentSearch
		}
		visibilityListStr := ctx.Query("visibility")
		if visibilityListStr != "" {
			visibilityList := []api.Visibility{}
			for _, visibility := range strings.Split(visibilityListStr, ",") {
				visibilityList = append(visibilityList, api.Visibility(visibility))
			}
			memoFind.VisibilityList = visibilityList
		}
		if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
			memoFind.Limit = &limit
		}
		if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
			memoFind.Offset = &offset
		}

		list, err := s.Store.FindMemoList(ctx, memoFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch memo list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(list))
	})

	rg.GET("/memo/:memoId", func(ctx *gin.Context) {

		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		memoFind := &api.MemoFind{
			ID: &memoID,
		}
		memo, err := s.Store.FindMemo(ctx, memoFind)
		if err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Memo ID not found: %d", memoID))
				return
			}
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to find memo by ID: %v", memoID))
			return
		}

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if memo.Visibility == api.Private {
			if !ok || memo.CreatorID != userID {
				ctx.String(http.StatusForbidden, "this memo is private only")
				return
			}
		} else if memo.Visibility == api.Protected {
			if !ok || !_ok {
				ctx.String(http.StatusForbidden, "this memo is protected, missing user in session")
				return
			}
		}
		ctx.JSON(http.StatusOK, composeResponse(memo))
	})

	rg.POST("/memo/:memoId/organizer", func(ctx *gin.Context) {

		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		memoOrganizerUpsert := &api.MemoOrganizerUpsert{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(memoOrganizerUpsert); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post memo organizer request")
			return
		}
		memoOrganizerUpsert.MemoID = memoID
		memoOrganizerUpsert.UserID = userID

		err = s.Store.UpsertMemoOrganizer(ctx, memoOrganizerUpsert)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to upsert memo organizer")
			return
		}

		memo, err := s.Store.FindMemo(ctx, &api.MemoFind{
			ID: &memoID,
		})
		if err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Memo ID not found: %d", memoID))
				return
			}
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to find memo by ID: %v", memoID))
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(memo))
	})

	rg.POST("/memo/:memoId/resource", func(ctx *gin.Context) {

		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		memoResourceUpsert := &api.MemoResourceUpsert{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(memoResourceUpsert); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post memo resource request")
			return
		}
		resourceFind := &api.ResourceFind{
			ID: &memoResourceUpsert.ResourceID,
		}
		resource, err := s.Store.FindResource(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch resource")
			return
		}
		if resource == nil {
			ctx.String(http.StatusBadRequest, "Resource not found")
			return
		} else if resource.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized to bind this resource")
			return
		}

		memoResourceUpsert.MemoID = memoID
		currentTs := time.Now().Unix()
		memoResourceUpsert.UpdatedTs = &currentTs
		if _, err := s.Store.UpsertMemoResource(ctx, memoResourceUpsert); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to upsert memo resource")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(resource))
	})

	rg.GET("/memo/:memoId/resource", func(ctx *gin.Context) {

		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		resourceFind := &api.ResourceFind{
			MemoID: &memoID,
		}
		resourceList, err := s.Store.FindResourceList(ctx, resourceFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch resource list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(resourceList))
	})

	rg.GET("/memo/stats", func(ctx *gin.Context) {

		normalStatus := api.Normal
		memoFind := &api.MemoFind{
			RowStatus: &normalStatus,
		}
		if creatorID, err := strconv.Atoi(ctx.Query("creatorId")); err == nil {
			memoFind.CreatorID = &creatorID
		}
		if memoFind.CreatorID == nil {
			ctx.String(http.StatusBadRequest, "Missing user id to find memo")
			return
		}

		_currentUserID, ok := ctx.Get(getUserIDContextKey())
		currentUserID, _ok := _currentUserID.(int)
		if !ok || !_ok {
			memoFind.VisibilityList = []api.Visibility{api.Public}
		} else {
			if *memoFind.CreatorID != currentUserID {
				memoFind.VisibilityList = []api.Visibility{api.Public, api.Protected}
			} else {
				memoFind.VisibilityList = []api.Visibility{api.Public, api.Protected, api.Private}
			}
		}

		list, err := s.Store.FindMemoList(ctx, memoFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch memo list")
			return
		}

		createdTsList := []int64{}
		for _, memo := range list {
			createdTsList = append(createdTsList, memo.CreatedTs)
		}
		ctx.JSON(http.StatusOK, composeResponse(createdTsList))
	})

	rg.GET("/memo/all", func(ctx *gin.Context) {

		memoFind := &api.MemoFind{}

		_userID, ok := ctx.Get(getUserIDContextKey())
		_, _ok := _userID.(int)
		if !ok || !_ok {
			memoFind.VisibilityList = []api.Visibility{api.Public}
		} else {
			memoFind.VisibilityList = []api.Visibility{api.Public, api.Protected}
		}

		pinnedStr := ctx.Query("pinned")
		if pinnedStr != "" {
			pinned := pinnedStr == "true"
			memoFind.Pinned = &pinned
		}
		tag := ctx.Query("tag")
		if tag != "" {
			contentSearch := "#" + tag + " "
			memoFind.ContentSearch = &contentSearch
		}
		visibilityListStr := ctx.Query("visibility")
		if visibilityListStr != "" {
			visibilityList := []api.Visibility{}
			for _, visibility := range strings.Split(visibilityListStr, ",") {
				visibilityList = append(visibilityList, api.Visibility(visibility))
			}
			memoFind.VisibilityList = visibilityList
		}
		if limit, err := strconv.Atoi(ctx.Query("limit")); err == nil {
			memoFind.Limit = &limit
		}
		if offset, err := strconv.Atoi(ctx.Query("offset")); err == nil {
			memoFind.Offset = &offset
		}

		// Only fetch normal status memos.
		normalStatus := api.Normal
		memoFind.RowStatus = &normalStatus

		list, err := s.Store.FindMemoList(ctx, memoFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch all memo list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(list))
	})

	rg.DELETE("/memo/:memoId", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("memoId")))
			return
		}

		memo, err := s.Store.FindMemo(ctx, &api.MemoFind{
			ID: &memoID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find memo")
			return
		}
		if memo.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		memoDelete := &api.MemoDelete{
			ID: memoID,
		}
		if err := s.Store.DeleteMemo(ctx, memoDelete); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Memo ID not found: %d", memoID))
				return
			}
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to delete memo ID: %v", memoID))
			return
		}
		ctx.JSON(http.StatusOK, true)
	})

	rg.DELETE("/memo/:memoId/resource/:resourceId", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		memoID, err := strconv.Atoi(ctx.Param("memoId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("Memo ID is not a number: %s", ctx.Param("memoId")))
			return
		}
		resourceID, err := strconv.Atoi(ctx.Param("resourceId"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("Resource ID is not a number: %s", ctx.Param("resourceId")))
			return
		}

		memo, err := s.Store.FindMemo(ctx, &api.MemoFind{
			ID: &memoID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find memo")
			return
		}
		if memo.CreatorID != userID {
			ctx.String(http.StatusUnauthorized, "Unauthorized")
			return
		}

		memoResourceDelete := &api.MemoResourceDelete{
			MemoID:     &memoID,
			ResourceID: &resourceID,
		}
		if err := s.Store.DeleteMemoResource(ctx, memoResourceDelete); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch resource list")
			return
		}
		ctx.JSON(http.StatusOK, true)
	})
}

func (s *Service) createMemoCreateActivity(ctx *gin.Context, memo *api.Memo) error {

	payload := api.ActivityMemoCreatePayload{
		Content:    memo.Content,
		Visibility: memo.Visibility.String(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: memo.CreatorID,
		Type:      api.ActivityMemoCreate,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
