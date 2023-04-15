package api

type UserSettingKey string

const (
	// UserSettingLocaleKey is the key type for user locale.
	UserSettingLocaleKey UserSettingKey = "locale"
	// UserSettingAppearanceKey is the key type for user appearance.
	UserSettingAppearanceKey UserSettingKey = "appearance"
	// UserSettingMemoVisibilityKey is the key type for user preference memo default visibility.
	UserSettingMemoVisibilityKey UserSettingKey = "memo-visibility"
)

func (key UserSettingKey) String() string {
	switch key {
	case UserSettingLocaleKey:
		return "locale"
	case UserSettingAppearanceKey:
		return "appearance"
	case UserSettingMemoVisibilityKey:
		return "memo-visibility"
	}
	return ""
}

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

type UserSettingFind struct {
	UserID int

	Key UserSettingKey `json:"key"`
}
