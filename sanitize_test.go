package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

func TestProblemClassifiesKnownErrors(t *testing.T) {
	validation := &userconfig.ValidationError{Field: "osc.manualPort", Err: errors.New("must be a valid port")}
	tests := []struct {
		name            string
		err             error
		currentRevision uint64
		code            string
		field           string
	}{
		{name: "validation", err: validation, code: ProblemValidation, field: "osc.manualPort"},
		{name: "platform", err: userconfig.ErrUnsupportedPlatform, code: ProblemUnsupportedPlatform},
		{name: "unknown plugin", err: plugins.ErrUnknownPlugin, code: ProblemNotFound},
		{name: "revision conflict", err: plugins.ErrConfigRevisionConflict, currentRevision: 9, code: ProblemConflict},
		{name: "settings conflict", err: userconfig.ErrConflict, currentRevision: 7, code: ProblemConflict},
		{name: "lifecycle", err: application.ErrInvalidLifecycle, code: ProblemUnavailable},
		{name: "deadline", err: context.DeadlineExceeded, code: ProblemTimeout},
		{name: "opaque", err: errors.New("token=do-not-expose"), code: ProblemInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := sanitizeProblem(test.err, test.currentRevision)
			if got.Code != test.code {
				t.Fatalf("Code = %q, want %q", got.Code, test.code)
			}
			if got.Field != test.field {
				t.Fatalf("Field = %q, want %q", got.Field, test.field)
			}
			if test.code == ProblemConflict && got.CurrentRevision != test.currentRevision {
				t.Fatalf("CurrentRevision = %d, want %d", got.CurrentRevision, test.currentRevision)
			}
		})
	}
}

func TestSanitizeProblemBoundsAndRedactsMessages(t *testing.T) {
	invalidUTF8 := string([]byte{'a', 0xff, 'b'})
	if got := boundedMessage(invalidUTF8); !utf8.ValidString(got) {
		t.Fatalf("boundedMessage() = invalid UTF-8 %q", got)
	}

	long := strings.Repeat("界", 300)
	if got := boundedMessage(long); !utf8.ValidString(got) || len(got) > 512 {
		t.Fatalf("boundedMessage() bytes=%d valid=%v, want valid UTF-8 no more than 512 bytes", len(got), utf8.ValidString(got))
	}

	secret := errors.New("token=abc123 config={\"password\":\"secret\"}")
	got := sanitizeProblem(secret, 0)
	if got.Code != ProblemInternal || got.Message != "internal operation failed" {
		t.Fatalf("sanitizeProblem(secret) = %#v, want generic internal problem", got)
	}
	if strings.Contains(got.Message, "token") || strings.Contains(got.Message, "config") {
		t.Fatalf("sanitizeProblem(secret) leaked payload in %q", got.Message)
	}
}
