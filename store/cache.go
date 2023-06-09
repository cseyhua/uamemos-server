package store

import (
	"fmt"

	"uamemos/api"
)

func getUserSettingCacheKey(userSetting userSettingRaw) string {
	return fmt.Sprintf("%d-%s", userSetting.UserID, userSetting.Key.String())
}

func getUserSettingFindCacheKey(userSettingFind *api.UserSettingFind) string {
	return fmt.Sprintf("%d-%s", userSettingFind.UserID, userSettingFind.Key.String())
}
