package email

import (
	"fmt"
	"html"
)

// RegistrationCodeMessage creates a verification code email with both HTML
// and plain text alternatives.
func RegistrationCodeMessage(to, code string) Message {
	subject := "验证码 - HotKey 热点监控"
	text := fmt.Sprintf("您的验证码是：%s，10分钟内有效。", code)
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body>
<p>您的验证码是：<strong>%s</strong></p>
<p>该验证码10分钟内有效，请勿泄露给他人。</p>
</body>
</html>`, html.EscapeString(code))

	return Message{
		To:      to,
		Subject: subject,
		HTML:    htmlContent,
		Text:    text,
	}
}

// PasswordResetCodeMessage creates a password reset code email with both HTML
// and plain text alternatives.
func PasswordResetCodeMessage(to, code string) Message {
	subject := "密码重置 - HotKey 热点监控"
	text := fmt.Sprintf("您的密码重置验证码是：%s，10分钟内有效。", code)
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body>
<p>您的密码重置验证码是：<strong>%s</strong></p>
<p>该验证码10分钟内有效，请勿泄露给他人。</p>
</body>
</html>`, html.EscapeString(code))

	return Message{
		To:      to,
		Subject: subject,
		HTML:    htmlContent,
		Text:    text,
	}
}

// PasswordChangedMessage creates a password change notification email.
// The displayName is HTML-escaped to prevent XSS in the HTML version.
func PasswordChangedMessage(to, displayName string) Message {
	subject := "密码已更改 - HotKey 热点监控"
	escapedName := html.EscapeString(displayName)
	text := fmt.Sprintf("%s，您的 HotKey 账户密码已成功更改。", displayName)
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body>
<p>%s，您好。</p>
<p>您的 HotKey 账户密码已成功更改。</p>
<p>如果这不是您本人操作，请立即联系客服。</p>
</body>
</html>`, escapedName)

	return Message{
		To:      to,
		Subject: subject,
		HTML:    htmlContent,
		Text:    text,
	}
}
