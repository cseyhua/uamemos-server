package api

import (
	"fmt"
	"uamemos/common"
)

type Role string

const (
	// Host is the HOST role.
	Host Role = "HOST"
	// Admin is the ADMIN role.
	Admin Role = "ADMIN"
	// NormalUser is the USER role.
	NormalUser Role = "USER"
)

func (e Role) String() string {
	switch e {
	case Host:
		return "HOST"
	case Admin:
		return "ADMIN"
	case NormalUser:
		return "USER"
	}
	return "USER"
}

type User struct {
	ID int `json:"id"`

	RowStatus RowStatus `json:"rowStatus"`
	CreatedTs int64     `json:"createdTs"`
	UpdatedTs int64     `json:"updatedTs"`

	// Domain specific fields
	Name            string         `json:"username"`
	Role            Role           `json:"role"`
	Email           string         `json:"email"`
	Nickname        string         `json:"nickname"`
	PasswordHash    string         `json:"-"`
	OpenID          string         `json:"openId"`
	AvatarURL       string         `json:"avatarUrl"`
	UserSettingList []*UserSetting `json:"userSettingList"`
}

type UserFind struct {
	ID        *int       `json:"id"`
	RowStatus *RowStatus `json:"rowStatus"`

	Name     *string `json:"username"`
	Role     *Role
	Email    *string `json:"email"`
	Nickname *string `json:"nickname"`
	OpenID   *string
}

type UserCreate struct {
	// Domain specific fields
	Name         string `json:"username"`
	Role         Role   `json:"role"`
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	Password     string `json:"password"`
	PasswordHash string
	OpenID       string
}

func (create UserCreate) Validate() error {
	if len(create.Name) < 3 {
		return fmt.Errorf("username is too short, minimum length is 3")
	}
	if len(create.Name) > 32 {
		return fmt.Errorf("username is too long, maximum length is 32")
	}
	if len(create.Password) < 3 {
		return fmt.Errorf("password is too short, minimum length is 6")
	}
	if len(create.Password) > 512 {
		return fmt.Errorf("password is too long, maximum length is 512")
	}
	if len(create.Nickname) > 64 {
		return fmt.Errorf("nickname is too long, maximum length is 64")
	}
	if create.Email != "" {
		if len(create.Email) > 256 {
			return fmt.Errorf("email is too long, maximum length is 256")
		}
		if !common.ValidateEmail(create.Email) {
			return fmt.Errorf("invalid email format")
		}
	}

	return nil
}
