package main

import (
	"context"
	"errors"
	"flag"
	"testing"
)

func TestCommandBoundary(t *testing.T) {
	for _, args := range [][]string{{"-help"}, {}} {
		if err := run(context.Background(), args); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("help=%v", err)
		}
	}
	for _, args := range [][]string{{"unknown"}, {"space", "add"}, {"migrate", "extra"}} {
		if err := run(context.Background(), args); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}
