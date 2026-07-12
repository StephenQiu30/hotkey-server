package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/StephenQiu30/hotkey-server/internal/model/enum"
)

// ErrorSpec defines the stable contract for an error code.
type ErrorSpec struct {
	HTTPStatus    int
	Message       string
	Retryable     bool
	SecurityEvent bool
}

// errorSpecs is the central registry of error code contracts.
// Every non-success code used in the application must have an entry here.
var errorSpecs = map[enum.ErrorCode]ErrorSpec{
	enum.ErrorCodeBadRequest:         {HTTPStatus: http.StatusBadRequest, Message: "请求参数错误"},
	enum.ErrorCodeUnauthorized:       {HTTPStatus: http.StatusUnauthorized, Message: "未授权访问"},
	enum.ErrorCodeForbidden:          {HTTPStatus: http.StatusForbidden, Message: "无权限访问"},
	enum.ErrorCodeNotFound:           {HTTPStatus: http.StatusNotFound, Message: "请求的资源不存在"},
	enum.ErrorCodeConflict:           {HTTPStatus: http.StatusConflict, Message: "资源冲突"},
	enum.ErrorCodeInternal:           {HTTPStatus: http.StatusInternalServerError, Message: "服务器内部错误"},
	enum.ErrorCodeRateLimited:        {HTTPStatus: http.StatusTooManyRequests, Message: "请求过于频繁，请稍后重试", Retryable: true},
	enum.ErrorCodeServiceUnavailable: {HTTPStatus: http.StatusServiceUnavailable, Message: "服务暂时不可用", Retryable: true},
	enum.ErrorCodeMethodNotAllowed:   {HTTPStatus: http.StatusMethodNotAllowed, Message: "请求方法不允许"},

	// Auth
	enum.ErrorCodeAuthInvalidInput:            {HTTPStatus: http.StatusBadRequest, Message: "认证输入无效"},
	enum.ErrorCodeInvalidCredentials:          {HTTPStatus: http.StatusUnauthorized, Message: "邮箱或密码错误", SecurityEvent: true},
	enum.ErrorCodeEmailAlreadyRegistered:      {HTTPStatus: http.StatusConflict, Message: "该邮箱已被注册"},
	enum.ErrorCodeVerificationInvalid:         {HTTPStatus: http.StatusBadRequest, Message: "验证码或票据无效"},
	enum.ErrorCodeVerificationExpired:         {HTTPStatus: http.StatusBadRequest, Message: "验证码或票据已过期"},
	enum.ErrorCodeVerificationTooManyAttempts: {HTTPStatus: http.StatusTooManyRequests, Message: "验证码错误次数过多", Retryable: true},
	enum.ErrorCodeVerificationSendTooFrequent: {HTTPStatus: http.StatusTooManyRequests, Message: "验证码发送过于频繁", Retryable: true},
	enum.ErrorCodeSessionExpired:              {HTTPStatus: http.StatusUnauthorized, Message: "会话已过期，请重新登录", SecurityEvent: true},
	enum.ErrorCodeSessionRevoked:              {HTTPStatus: http.StatusUnauthorized, Message: "会话已撤销", SecurityEvent: true},
	enum.ErrorCodeTokenInvalid:                {HTTPStatus: http.StatusUnauthorized, Message: "令牌无效", SecurityEvent: true},
	enum.ErrorCodeTokenReused:                 {HTTPStatus: http.StatusUnauthorized, Message: "令牌已被重复使用", SecurityEvent: true},
	enum.ErrorCodeAccountDisabled:             {HTTPStatus: http.StatusForbidden, Message: "账户已被禁用", SecurityEvent: true},
	enum.ErrorCodePasswordPolicyViolation:     {HTTPStatus: http.StatusBadRequest, Message: "密码不符合安全策略"},
}

// GetErrorSpec returns the ErrorSpec for the given code, or a generic fallback.
func GetErrorSpec(code enum.ErrorCode) ErrorSpec {
	if spec, ok := errorSpecs[code]; ok {
		return spec
	}
	return ErrorSpec{
		HTTPStatus: http.StatusInternalServerError,
		Message:    "服务器内部错误",
	}
}

// AppError carries stable error metadata for the HTTP responder.
// Callers create via NewAppError, which always populates HTTPStatus and Message
// from the central errorSpecs registry. Never pass internal cause text to clients.
type AppError struct {
	Code       enum.ErrorCode
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewAppError creates an application error with a stable external contract.
// The HTTP status and safe public message are looked up from the central
// errorSpecs registry by code. The cause is for internal logging only and is
// never returned to the client.
func NewAppError(code enum.ErrorCode, cause error) *AppError {
	spec := GetErrorSpec(code)
	return &AppError{
		Code:       code,
		HTTPStatus: spec.HTTPStatus,
		Message:    spec.Message,
		Cause:      cause,
	}
}

// Deprecated: oldNewAppError has been removed. Use NewAppError instead.

// errorCodeToHTTPStatus maps a stable ErrorCode to its HTTP status code via the spec registry.
func errorCodeToHTTPStatus(code enum.ErrorCode) int {
	return GetErrorSpec(code).HTTPStatus
}

// requestIDFromContext extracts the request ID from the gin context.
func requestIDFromContext(c *gin.Context) string {
	if value, ok := c.Get("request_id"); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return c.GetHeader("X-Request-Id")
}

// ErrorBody is the deprecated error response type kept for swagger doc compatibility.
// New code should use vo.ResponseBody directly.
type ErrorBody struct {
	Code      int            `json:"code"`
	ErrorCode enum.ErrorCode `json:"error_code"`
	Data      any            `json:"data"`
}
