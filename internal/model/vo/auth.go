package vo

// UserData is the JSON representation of a user (nested inside ResponseBody.Data).
type UserData struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// LoginData is the JSON representation of a login response (nested inside ResponseBody.Data).
type LoginData struct {
	User  UserData `json:"user"`
	Token string   `json:"token"`
}
