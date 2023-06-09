package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"uamemos/api"
	"uamemos/common"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
)

func (s *Service) registerSystemRoutes(rg *gin.RouterGroup) {
	rg.GET("/ping", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, composeResponse(s.Profile))
	})
	rg.GET("/status", func(ctx *gin.Context) {
		hostUserType := api.Host
		hostUserFind := api.UserFind{
			Role: &hostUserType,
		}
		hostUser, err := s.Store.FindUser(ctx, &hostUserFind)
		if err != nil && common.ErrorCode(err) != common.NotFound {
			ctx.String(http.StatusInternalServerError, "Failed to find host user")
			return
		}
		if hostUser != nil {
			hostUser.OpenID = ""
			hostUser.Email = ""
		}
		systemStatus := api.SystemStatus{
			Host:               hostUser,
			Profile:            *s.Profile,
			DBSize:             0,
			AllowSignUp:        false,
			IgnoreUpgrade:      false,
			DisablePublicMemos: false,
			AdditionalStyle:    "",
			AdditionalScript:   "",
			CustomizedProfile: api.CustomizedProfile{
				Name:        "uamemos",
				LogoURL:     "",
				Description: "",
				Locale:      "zh",
				Appearance:  "system",
				ExternalURL: "",
			},
			StorageServiceID: api.DatabaseStorage,
			LocalStoragePath: "",
		}
		systemSettingList, err := s.Store.FindSystemSettingList(ctx, &api.SystemSettingFind{})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find system setting list")
			return
		}
		for _, systemSetting := range systemSettingList {
			if systemSetting.Name == api.SystemSettingServiceIDName || systemSetting.Name == api.SystemSettingSecretSessionName || systemSetting.Name == api.SystemSettingOpenAIConfigName {
				continue
			}
			var baseValue any
			err := json.Unmarshal([]byte(systemSetting.Value), &baseValue)
			if err != nil {
				log.Warn("Failed to unmarshal system setting value", zap.String("setting name", systemSetting.Name.String()))
				continue
			}

			if systemSetting.Name == api.SystemSettingAllowSignUpName {
				systemStatus.AllowSignUp = baseValue.(bool)
			} else if systemSetting.Name == api.SystemSettingIgnoreUpgradeName {
				systemStatus.IgnoreUpgrade = baseValue.(bool)
			} else if systemSetting.Name == api.SystemSettingDisablePublicMemosName {
				systemStatus.DisablePublicMemos = baseValue.(bool)
			} else if systemSetting.Name == api.SystemSettingAdditionalStyleName {
				systemStatus.AdditionalStyle = baseValue.(string)
			} else if systemSetting.Name == api.SystemSettingAdditionalScriptName {
				systemStatus.AdditionalScript = baseValue.(string)
			} else if systemSetting.Name == api.SystemSettingCustomizedProfileName {
				customizedProfile := api.CustomizedProfile{}
				err := json.Unmarshal([]byte(systemSetting.Value), &customizedProfile)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to unmarshal system setting customized profile value")
					return
				}
				systemStatus.CustomizedProfile = customizedProfile
			} else if systemSetting.Name == api.SystemSettingStorageServiceIDName {
				systemStatus.StorageServiceID = int(baseValue.(float64))
			} else if systemSetting.Name == api.SystemSettingLocalStoragePathName {
				systemStatus.LocalStoragePath = baseValue.(string)
			}
		}
		userID, ok := ctx.Get(getUserIDContextKey())
		if ok {
			userID, _ := userID.(int)
			user, err := s.Store.FindUser(ctx, &api.UserFind{
				ID: &userID,
			})
			if err != nil {
				ctx.String(http.StatusInternalServerError, "Failed to find user")
				return
			}
			if user != nil && user.Role == api.Host {
				fi, err := os.Stat(s.Profile.DSN)
				if err != nil {
					ctx.String(http.StatusInternalServerError, "Failed to read database fileinfo")
					return
				}
				systemStatus.DBSize = fi.Size()
			}
		}
		ctx.JSON(http.StatusOK, composeResponse(systemStatus))
	})
	rg.POST("/system/setting", func(ctx *gin.Context) {
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

		systemSettingUpsert := &api.SystemSettingUpsert{}
		if err := json.NewDecoder(ctx.Request.Body).Decode(systemSettingUpsert); err != nil {
			ctx.String(http.StatusBadRequest, "Malformatted post system setting request")
			return
		}
		if err := systemSettingUpsert.Validate(); err != nil {
			ctx.String(http.StatusBadRequest, "system setting invalidate")
			return
		}

		systemSetting, err := s.Store.UpsertSystemSetting(ctx, systemSettingUpsert)
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to upsert system setting")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(systemSetting))
	})

	rg.GET("/system/setting", func(ctx *gin.Context) {
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

		systemSettingList, err := s.Store.FindSystemSettingList(ctx, &api.SystemSettingFind{})
		if err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to find system setting list")
			return
		}
		ctx.JSON(http.StatusOK, composeResponse(systemSettingList))
	})

	rg.POST("/system/vacuum", func(ctx *gin.Context) {
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

		if err := s.Store.Vacuum(ctx); err != nil {
			ctx.String(http.StatusInternalServerError, "Failed to vacuum database")
			return
		}
		ctx.JSON(http.StatusOK, true)
	})

}

func (s *Service) getSystemServiceID(ctx context.Context) (string, error) {
	serviceID, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{
		Name: api.SystemSettingServiceIDName,
	})
	if err != nil && common.ErrorCode(err) != common.NotFound {
		return "", err
	}
	if serviceID == nil || serviceID.Value == "" {
		serviceID, err = s.Store.UpsertSystemSetting(ctx, &api.SystemSettingUpsert{
			Name:  api.SystemSettingServiceIDName,
			Value: uuid.NewString(),
		})
		if err != nil {
			return "", err
		}
	}
	return serviceID.Value, nil
}

func (s *Service) getSystemSecretSessionName(ctx context.Context) (string, error) {
	secretSessionNameValue, err := s.Store.FindSystemSetting(ctx, &api.SystemSettingFind{
		Name: api.SystemSettingSecretSessionName,
	})
	if err != nil && common.ErrorCode(err) != common.NotFound {
		return "", err
	}
	if secretSessionNameValue == nil || secretSessionNameValue.Value == "" {
		secretSessionNameValue, err = s.Store.UpsertSystemSetting(ctx, &api.SystemSettingUpsert{
			Name:  api.SystemSettingSecretSessionName,
			Value: uuid.NewString(),
		})
		if err != nil {
			return "", err
		}
	}
	return secretSessionNameValue.Value, nil
}
