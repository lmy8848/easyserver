package auth

import (
	"easyserver/internal/infra/errx"
)

var (
	// ErrInvalidCredentials 表示用户名或密码错误
	ErrInvalidCredentials = errx.NewSentinel(errx.KindUnauthorized, 40101, "用户名或密码错误")

	// ErrInvalidToken 表示认证令牌无效
	ErrInvalidToken = errx.NewSentinel(errx.KindUnauthorized, 40102, "无效的认证令牌")

	// ErrAccountLocked 表示账号已被锁定
	ErrAccountLocked = errx.NewSentinel(errx.KindForbidden, 40301, "账号已被锁定")

	// ErrOldPasswordInvalid 表示旧密码/当前密码验证失败
	ErrOldPasswordInvalid = errx.NewSentinel(errx.KindBadRequest, 40001, "当前密码不正确")

	// ErrSamePassword 表示新密码不能与旧密码相同
	ErrSamePassword = errx.NewSentinel(errx.KindBadRequest, 40002, "新密码不能与旧密码相同")

	// ErrSameUsername 表示新用户名不能与当前用户名相同
	ErrSameUsername = errx.NewSentinel(errx.KindBadRequest, 40003, "新用户名不能与当前用户名相同")

	// ErrInvalidUsername 表示用户名格式或长度无效
	ErrInvalidUsername = errx.NewSentinel(errx.KindBadRequest, 40004, "用户名格式无效（3-32位字母、数字、下划线或短横线）")

	// ErrUsernameTaken 表示该用户名已存在
	ErrUsernameTaken = errx.NewSentinel(errx.KindConflict, 40901, "该用户名已存在")

	// ErrUserNotFound 表示用户不存在
	ErrUserNotFound = errx.NewSentinel(errx.KindNotFound, 40401, "用户不存在")

	// ErrInvalidPassword 表示密码不符合安全要求（8-72位，含大小写字母和数字，非弱密码）
	ErrInvalidPassword = errx.NewSentinel(errx.KindBadRequest, 40010, "密码格式不符合要求（8-72位，须包含大小写字母和数字，且不能是弱密码）")

	// 2FA / TOTP 相关
	ErrNoPendingTOTP = errx.NewSentinel(errx.KindBadRequest, 40020, "no pending TOTP secret found")

	// 扫码登录相关
	ErrQRNotPending = errx.NewSentinel(errx.KindBadRequest, 40021, "二维码已失效或已确认")
	ErrQRExpired    = errx.NewSentinel(errx.KindBadRequest, 40022, "二维码已过期")
)
