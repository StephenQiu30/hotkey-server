package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

// UserEntityToDTO converts a User entity to a User DTO.
func UserEntityToDTO(u entity.User) dto.User {
	return dto.User{
		ID:           u.ID,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		DisplayName:  u.DisplayName,
		Status:       u.Status,
		PlanType:     u.PlanType,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// UserDTOToVO converts a User DTO to a UserData VO.
func UserDTOToVO(u dto.User) vo.UserData {
	return vo.UserData{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}

// LoginDTOToVO converts a User DTO + token to a LoginData VO.
func LoginDTOToVO(u dto.User, token string) vo.LoginData {
	return vo.LoginData{
		User:  UserDTOToVO(u),
		Token: token,
	}
}
