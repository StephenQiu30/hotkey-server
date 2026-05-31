package auth

import (
	"github.com/StephenQiu30/hotkey-server/internal/domain/user"
	"github.com/gin-gonic/gin"
)

const currentUserKey = "currentUser"

func SetCurrentUser(c *gin.Context, account user.User) {
	c.Set(currentUserKey, account)
}

func CurrentUser(c *gin.Context) (user.User, bool) {
	value, exists := c.Get(currentUserKey)
	if !exists {
		return user.User{}, false
	}
	account, ok := value.(user.User)
	return account, ok
}

func userResponse(account user.User) gin.H {
	return gin.H{
		"id":          account.ID,
		"email":       account.Email,
		"role":        account.Role,
		"status":      account.Status,
		"timezone":    account.Timezone,
		"dailySendAt": account.DailySendAt,
		"wechatBound": account.WeChatOpenID != "",
	}
}
