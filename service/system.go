package service

import (
	"context"
	"uamemos/api"
	"uamemos/common"

	"github.com/google/uuid"
)

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
