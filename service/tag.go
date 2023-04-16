package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"

	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
)

func (s *Service) registerTagRoutes(rg *gin.RouterGroup) {
	rg.POST("/tag", func(ctx *gin.Context) {
		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		tagUpsert := &api.TagUpsert{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(tagUpsert); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post tag request")
			return
		}
		if tagUpsert.Name == "" {
			ctx.String(http.StatusBadRequest, "Tag name shouldn't be empty")
			return
		}

		tagUpsert.CreatorID = userID
		tag, err := s.Store.UpsertTag(ctx, tagUpsert)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to upsert tag")
			return
		}
		if err := s.createTagCreateActivity(ctx, tag); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(tag.Name))
	})

	rg.GET("/tag", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusBadRequest, "Missing user id to find tag")
			return
		}

		tagFind := &api.TagFind{
			CreatorID: userID,
		}
		tagList, err := s.Store.FindTagList(ctx, tagFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find tag list")
			return
		}

		tagNameList := []string{}
		for _, tag := range tagList {
			tagNameList = append(tagNameList, tag.Name)
		}
		ctx.JSON(http.StatusOK, composeResponse(tagNameList))
	})

	rg.GET("/tag/suggestion", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusBadRequest, "Missing user session")
			return
		}
		contentSearch := "#"
		normalRowStatus := api.Normal
		memoFind := api.MemoFind{
			CreatorID:     &userID,
			ContentSearch: &contentSearch,
			RowStatus:     &normalRowStatus,
		}

		memoList, err := s.Store.FindMemoList(ctx, &memoFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find memo list")
			return
		}

		tagFind := &api.TagFind{
			CreatorID: userID,
		}
		existTagList, err := s.Store.FindTagList(ctx, tagFind)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find tag list")
			return
		}
		tagNameList := []string{}
		for _, tag := range existTagList {
			tagNameList = append(tagNameList, tag.Name)
		}

		tagMapSet := make(map[string]bool)
		for _, memo := range memoList {
			for _, tag := range findTagListFromMemoContent(memo.Content) {
				if !slices.Contains(tagNameList, tag) {
					tagMapSet[tag] = true
				}
			}
		}
		tagList := []string{}
		for tag := range tagMapSet {
			tagList = append(tagList, tag)
		}
		sort.Strings(tagList)
		ctx.JSON(http.StatusOK, composeResponse(tagList))
	})

	rg.POST("/tag/delete", func(ctx *gin.Context) {

		_userID, ok := ctx.Get(getUserIDContextKey())
		userID, _ok := _userID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}

		tagDelete := &api.TagDelete{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(tagDelete); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post tag request")
			return
		}
		if tagDelete.Name == "" {
			ctx.String(http.StatusBadRequest, "Tag name shouldn't be empty")
			return
		}

		tagDelete.CreatorID = userID
		if err := s.Store.DeleteTag(ctx, tagDelete); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("Tag name not found: %s", tagDelete.Name))
				return
			}
			ctx.String(http.StatusInternalServerError, fmt.Sprintf("Failed to delete tag name: %v", tagDelete.Name))
			return
		}
		ctx.JSON(http.StatusOK, true)
	})
}

var tagRegexp = regexp.MustCompile(`#([^\s#]+)`)

func findTagListFromMemoContent(memoContent string) []string {
	tagMapSet := make(map[string]bool)
	matches := tagRegexp.FindAllStringSubmatch(memoContent, -1)
	for _, v := range matches {
		tagName := v[1]
		tagMapSet[tagName] = true
	}

	tagList := []string{}
	for tag := range tagMapSet {
		tagList = append(tagList, tag)
	}
	sort.Strings(tagList)
	return tagList
}

func (s *Service) createTagCreateActivity(ctx *gin.Context, tag *api.Tag) error {

	payload := api.ActivityTagCreatePayload{
		TagName: tag.Name,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: tag.CreatorID,
		Type:      api.ActivityTagCreate,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
