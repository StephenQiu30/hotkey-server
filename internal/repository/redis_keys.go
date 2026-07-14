package repository

import "fmt"

const (
	redisSessionKeyFmt = "session:%d"
	redisUserTokensFmt = "user_tokens:%d"
	redisRevokedJtiFmt = "revoked_jti:%s"
	redisUserInfoFmt   = "user_info:%d"
)

func sessionKey(sessionID int64) string   { return fmt.Sprintf(redisSessionKeyFmt, sessionID) }
func userTokensKey(userID int64) string   { return fmt.Sprintf(redisUserTokensFmt, userID) }
func revokedJtiKey(jti string) string     { return fmt.Sprintf(redisRevokedJtiFmt, jti) }
func userInfoKey(userID int64) string     { return fmt.Sprintf(redisUserInfoFmt, userID) }
