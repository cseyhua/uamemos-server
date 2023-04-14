package service

import (
	"encoding/json"
	"net/http"
	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) registerAuthRoutes(rg *gin.RouterGroup, secret string) {
	rg.POST("/auth/signup", func(ctx *gin.Context) {
		signup := &api.SignUp{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(signup); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted signup request")
			return
		}
		userCreate := &api.UserCreate{
			Name:     signup.Name,
			Role:     api.NormalUser,
			Nickname: signup.Name,
			Password: signup.Pass,
			OpenID:   common.GenUUID(),
		}
		hostUserType := api.Host
		// 查找所有host用户
		existedHostUsers, err := s.Store.FindUserList(ctx, &api.UserFind{
			Role: &hostUserType,
		})
		if err != nil {
			ctx.String(http.StatusBadRequest, "Failed to find users")
			return
		}
		// 看host用户是否存在
		if len(existedHostUsers) == 0 {
			// 第一个注册的用户，设置为host用户
			userCreate.Role = api.Host
		} else {
			// 查找运行注册设置
			allowSignUpSetting, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{
				Name: api.SystemSettingAllowSignUpName,
			})
			if err != nil && common.ErrorCode(err) != common.NotFound {
				ctx.String(http.StatusInternalServerError, "Failed to find system setting")
				return
			}
			allowSignUpSettingValue := false
			if allowSignUpSetting != nil {
				err = json.Unmarshal([]byte(allowSignUpSetting.Value), &allowSignUpSettingValue)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to unmarshal system setting allow signup")
					return
				}
			}
			if !allowSignUpSettingValue {
				ctx.String(http.StatusUnauthorized, "signup is disabled")
				return
			}
		}
		if err := userCreate.Validate(); err != nil {
			ctx.String(http.StatusBadRequest, "Invalid user create format")
			return
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(signup.Pass), bcrypt.DefaultCost)
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
		if err := GenerateTokensAndSetCookies(ctx, user, secret); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to generate tokens")
			return
		}
		if err := s.createUserAuthSignUpActivity(ctx, user); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(user))
	})
}

func (s *Service) createUserAuthSignUpActivity(ctx *gin.Context, user *api.User) error {
	payload := api.ActivityUserAuthSignUpPayload{
		Username: user.Name,
		IP:       ctx.Request.Header.Get("X-Forward-For"),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: user.ID,
		Type:      api.ActivityUserAuthSignUp,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
}
