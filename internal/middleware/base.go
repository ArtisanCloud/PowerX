package middleware

type Option struct {
	Public      []string
	WhiteList   []string
	DisableAuth bool
}

type OptionFunc func(opt *Option)

// WithPublicPrefix 公开访问前缀
func WithPublicPrefix(path ...string) OptionFunc {
	return func(opt *Option) {
		opt.Public = path
	}
}

// WithWhiteListPrefix 无需权限验证前缀
func WithWhiteListPrefix(path ...string) OptionFunc {
	return func(opt *Option) {
		opt.WhiteList = path
	}
}

func DisableToken(b bool) func(opt *Option) {
	return func(opt *Option) {
		opt.DisableAuth = b
	}
}
