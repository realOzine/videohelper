package apperr

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWrappedErrorPreservesCauseWithoutExposingIt(t *testing.T) {
	cause := errors.New("password=secret")
	app := Wrap(Unavailable, "unavailable", cause)
	err := fmt.Errorf("operation: %w", app)
	if KindOf(err) != Unavailable || !errors.Is(err, cause) {
		t.Fatal("wrapped kind or cause was lost")
	}
	var target *Error
	if !errors.As(err, &target) || target != app {
		t.Fatal("errors.As cannot recover the application error")
	}
	if strings.Contains(err.Error(), "secret") || target.Msg == "" {
		t.Fatal("public message exposes cause or is missing")
	}
	if KindOf(cause) != Internal || KindOf(nil) != Internal {
		t.Fatal("ordinary errors must be internal")
	}
	if got := New(NotFound, "not_found", "视频不存在").Error(); got != "视频不存在" {
		t.Fatalf("public message = %q", got)
	}
}
