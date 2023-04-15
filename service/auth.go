package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

func (s *Service) registerAuthRoutes(rg *gin.RouterGroup, secret string) {
	rg.POST("/auth/signin", func(ctx *gin.Context) {
		signin := &api.SignIn{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(signin); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted signup request")
			return
		}
		userFind := &api.UserFind{
			Name: &signin.Name,
		}
		user, err := s.Store.FindUser(ctx, userFind)
		if err != nil && common.ErrorCode(err) != common.NotFound {
			ctx.String(http.StatusInternalServerError, "Incorrect login credentials, please try again")
			return
		}
		if user == nil {
			ctx.String(http.StatusUnauthorized, "Incorrect login credentials, please try again")
			return
		} else if user.RowStatus == api.Archived {
			ctx.String(http.StatusForbidden, fmt.Sprintf("User has been archived with username %s", signin.Name))
			return
		}

		// Compare the stored hashed password, with the hashed version of the password that was received.
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(signin.Pass)); err != nil {
			// If the two passwords don't match, return a 401 status.
			ctx.String(http.StatusUnauthorized, "Incorrect login credentials, please try again")
			return
		}

		if err := GenerateTokensAndSetCookies(ctx, user, secret); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to generate tokens")
			return
		}
		if err := s.createUserAuthSignInActivity(ctx, user); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to create activity")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(user))
	})

	rg.POST("/auth/signup", func(ctx *gin.Context) {
		signup := &api.SignUp{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(&signup); err != nil {
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
		existedHostUsers, err := s.Store.FindUserList(ctx, &api.UserFind{
			Role: &hostUserType,
		})
		if err != nil {
			ctx.String(http.StatusBadRequest, "Failed to find users")
			return
		}
		if len(existedHostUsers) == 0 {
			userCreate.Role = api.Host
		} else {
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
				ctx.String(http.StatusUnauthorized, "Signup is disabled")
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

func (s *Service) createUserAuthSignInActivity(ctx *gin.Context, user *api.User) error {
	payload := api.ActivityUserAuthSignInPayload{
		UserID: user.ID,
		IP:     ctx.Request.Header.Get("X-Forward-For"),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "failed to marshal activity payload")
	}
	activity, err := s.Store.CreateActivity(ctx, &api.ActivityCreate{
		CreatorID: user.ID,
		Type:      api.ActivityUserAuthSignIn,
		Level:     api.ActivityInfo,
		Payload:   string(payloadBytes),
	})
	if err != nil || activity == nil {
		return errors.Wrap(err, "failed to create activity")
	}
	return err
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
