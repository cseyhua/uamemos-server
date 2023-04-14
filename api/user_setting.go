package api

type UserSettingKey string

type UserSetting struct {
	UserID int
	Key    UserSettingKey `json:"key"`
	// Value is a JSON string with basic value
	Value string `json:"value"`
}

var (
	UserSettingLocaleValue         = []string{"en", "zh", "vi", "fr", "nl", "sv", "de", "es", "uk", "ru", "it", "hant", "tr", "ko", "sl"}
	UserSettingAppearanceValue     = []string{"system", "light", "dark"}
	UserSettingMemoVisibilityValue = []Visibility{Private, Protected, Public}
)
