package convert

import (
	"github.com/StephenQiu30/hotkey-server/internal/model/dto"
	"github.com/StephenQiu30/hotkey-server/internal/model/entity"
	"github.com/StephenQiu30/hotkey-server/internal/model/vo"
	"github.com/StephenQiu30/hotkey-server/internal/service"
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

// AuthResultToLoginVO converts an AuthResult to a LoginData VO using the
// access token from the session tokens.
func AuthResultToLoginVO(r *service.AuthResult) vo.LoginData {
	return vo.LoginData{
		User:  UserDTOToVO(r.User),
		Token: r.Tokens.AccessToken,
	}
}

// TokensToAuthTokenData converts SessionTokens to AuthTokenData VO.
func TokensToAuthTokenData(t *service.SessionTokens) vo.AuthTokenData {
	return vo.AuthTokenData{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}
}

// UserDTOToAuthenticatedUserVO converts a User DTO to an AuthenticatedUserData VO.
func UserDTOToAuthenticatedUserVO(u dto.User) vo.AuthenticatedUserData {
	return vo.AuthenticatedUserData{
		ID:              u.ID,
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		Status:          u.Status,
		PlanType:        u.PlanType,
		EmailVerifiedAt: u.EmailVerifiedAt,
		CreatedAt:       u.CreatedAt,
	}
}
