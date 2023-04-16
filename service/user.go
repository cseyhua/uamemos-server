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
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) registerUserRoutes(rg *gin.RouterGroup) {

	rg.POST("/user", func(ctx *gin.Context) {
		userID, ok := ctx.Get(getUserIDContextKey())

		_userID, _ := userID.(int)

		if !ok {
			ctx.String(http.StatusUnauthorized, "Missing auth session")
			return
		}
		currentUser, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &_userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user by id")
			return
		}
		if currentUser.Role != api.Host {
			ctx.String(http.StatusUnauthorized, "Only Host user can create member")
			return
		}
		userCreate := &api.UserCreate{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(userCreate); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post user request")
			return
		}
		if userCreate.Role == api.Host {
			ctx.String(http.StatusForbidden, "Could not create host user")
			return
		}
		userCreate.OpenID = common.GenUUID()
		if err := userCreate.Validate(); err != nil {
			ctx.String(http.StatusBadRequest, "Invalid user create format")
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(userCreate.Password), bcrypt.DefaultCost)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to generate password hash")
			return
		}
		userCreate.PasswordHash = string(passwordHash)
		user, err := s.Store.CreateUser(ctx, userCreate)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create user")
			return
		}
		if err := s.createUserCreateActivity(ctx, user); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(user))

	})

	rg.GET("/user", func(ctx *gin.Context) {
		userList, err := s.Store.FindUserList(ctx, &api.UserFind{})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch user list")
			return
		}

		for _, user := range userList {
			// data desensitize
			user.OpenID = ""
			user.Email = ""
		}
		ctx.JSON(http.StatusOK, composeResponse(userList))
	})

	rg.POST("/user/setting", func(ctx *gin.Context) {
		userID, ok := ctx.Get(getUserIDContextKey())
		_userID, _ := userID.(int)
		if !ok {
			ctx.String(http.StatusUnauthorized, "Missing auth session")
			return
		}

		userSettingUpsert := &api.UserSettingUpsert{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(userSettingUpsert); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post user setting upsert request")
			return
		}
		if err := userSettingUpsert.Validate(); err != nil {
			ctx.String(http.StatusBadRequest, "Invalid user setting format")
			return
		}

		userSettingUpsert.UserID = _userID
		userSetting, err := s.Store.UpsertUserSetting(ctx, userSettingUpsert)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to upsert user setting")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(userSetting))
	})

	rg.GET("/user/me", func(ctx *gin.Context) {
		userID, ok := ctx.Get(getUserIDContextKey())
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
	rg.GET("/user/:id", func(ctx *gin.Context) {
		id, err := strconv.Atoi(ctx.Param("id"))
		if err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted user id")
			return
		}

		user, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &id,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to fetch user")
			return
		}

		if user != nil {
			// data desensitize
			user.OpenID = ""
			user.Email = ""
		}
		ctx.JSON(http.StatusOK, composeResponse(user))
	})
	rg.PATCH("/user/:id", func(ctx *gin.Context) {
		userID, err := strconv.Atoi(ctx.Param("id"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("id")))
			return
		}
		currentUserID, ok := ctx.Get(getUserIDContextKey())
		_currentUserID, _ := currentUserID.(int)
		if !ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		currentUser, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &_currentUserID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		if currentUser == nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("Current session user not found with ID: %d", currentUserID))
			return
		} else if currentUser.Role != api.Host && _currentUserID != userID {
			ctx.String(http.StatusForbidden, "Access forbidden for current session user")
			return
		}

		currentTs := time.Now().Unix()
		userPatch := &api.UserPatch{
			UpdatedTs: &currentTs,
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(userPatch); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted patch user request")
			return
		}
		userPatch.ID = userID

		if userPatch.Password != nil && *userPatch.Password != "" {
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(*userPatch.Password), bcrypt.DefaultCost)
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to generate password hash")
				return
			}
			passwordHashStr := string(passwordHash)
			userPatch.PasswordHash = &passwordHashStr
		}

		if userPatch.ResetOpenID != nil && *userPatch.ResetOpenID {
			openID := common.GenUUID()
			userPatch.OpenID = &openID
		}

		if err := userPatch.Validate(); err != nil {
			ctx.String(http.StatusBadRequest, "Invalid user patch format")
			return
		}

		user, err := s.Store.PatchUser(ctx, userPatch)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to patch user")
			return
		}

		userSettingList, err := s.Store.FindUserSettingList(ctx, &api.UserSettingFind{
			UserID: userID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find userSettingList")
			return
		}
		user.UserSettingList = userSettingList
		ctx.JSON(http.StatusOK, composeResponse(user))
	})
	rg.DELETE("/user/:id", func(ctx *gin.Context) {
		currentUserID, ok := ctx.Get(getUserIDContextKey())
		_currentUserID, _ok := currentUserID.(int)
		if !ok || !_ok {
			ctx.String(http.StatusUnauthorized, "Missing user in session")
			return
		}
		currentUser, err := s.Store.FindUser(ctx, &api.UserFind{
			ID: &_currentUserID,
		})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find user")
			return
		}
		if currentUser == nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("Current session user not found with ID: %d", currentUserID))
			return
		} else if currentUser.Role != api.Host {
			ctx.String(http.StatusForbidden, "Access forbidden for current session user")
			return
		}

		userID, err := strconv.Atoi(ctx.Param("id"))
		if err != nil {
			ctx.String(http.StatusBadRequest, fmt.Sprintf("ID is not a number: %s", ctx.Param("id")))
			return
		}

		userDelete := &api.UserDelete{
			ID: userID,
		}
		if err := s.Store.DeleteUser(ctx, userDelete); err != nil {
			if common.ErrorCode(err) == common.NotFound {
				ctx.String(http.StatusNotFound, fmt.Sprintf("User ID not found: %d", userID))
				return
			}
			ctx.String(http.StatusInternalServerError, "Failed to delete user")
			return
		}

		ctx.JSON(http.StatusOK, true)
	})
}

func (s *Service) createUserCreateActivity(ctx *gin.Context, user *api.User) error {
	payload := api.ActivityUserCreatePayload{
		UserID:   user.ID,
		Username: user.Name,
		Role:     user.Role,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: user.ID,
		Type:      api.ActivityUserCreate,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
