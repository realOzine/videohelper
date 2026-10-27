package apperr

import "errors"

type Kind int

const (
	Invalid Kind = iota + 1
	NotFound
	Unavailable
	Internal
)

type Error struct {
	Kind Kind
	Code string
	Msg  string
	Err  error
}

func New(k Kind, code, msg string) *Error {
	return &Error{Kind: k, Code: code, Msg: msg}
}

func Wrap(k Kind, code string, err error) *Error {
	return &Error{Kind: k, Code: code, Msg: message(k), Err: err}
}

// Error exposes only the public message; callers can log the cause through Unwrap.
func (e *Error) Error() string {
	if e.Msg != "" {
		return e.Msg
	}
	return message(e.Kind)
}

func (e *Error) Unwrap() error { return e.Err }

func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) && e != nil {
		return e.Kind
	}
	return Internal
}

func message(k Kind) string {
	switch k {
	case Invalid:
		return "参数不合法"
	case NotFound:
		return "资源不存在"
	case Unavailable:
		return "服务暂时不可用"
	default:
		return "处理失败"
	}
}
