package main

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/wzhqwq/vrcft-go/internal/application"
	"github.com/wzhqwq/vrcft-go/internal/plugins"
	"github.com/wzhqwq/vrcft-go/internal/userconfig"
)

const maxProblemMessageBytes = 512

// boundedMessage returns a valid UTF-8 prefix that fits the public-message
// bound. It replaces malformed source bytes before calculating the boundary.
func boundedMessage(message string) string {
	message = stringsToValidUTF8(message)
	if len(message) <= maxProblemMessageBytes {
		return message
	}

	end := 0
	for _, runeValue := range message {
		runeBytes := utf8.RuneLen(runeValue)
		if end+runeBytes > maxProblemMessageBytes {
			break
		}
		end += runeBytes
	}
	return message[:end]
}

func stringsToValidUTF8(value string) string {
	if utf8.ValidString(value) {
		return value
	}
	result := make([]rune, 0, len(value))
	for len(value) > 0 {
		runeValue, size := utf8.DecodeRuneInString(value)
		result = append(result, runeValue)
		value = value[size:]
	}
	return string(result)
}

// sanitizeProblem maps errors to the stable frontend contract. Only known,
// expected validation messages are preserved; unexpected errors deliberately
// receive a generic message so wrapped data cannot cross the Wails boundary.
func sanitizeProblem(err error, currentRevision uint64) Problem {
	var validation *userconfig.ValidationError
	switch {
	case errors.As(err, &validation):
		return Problem{
			Code:    ProblemValidation,
			Message: boundedMessage(validation.Error()),
			Field:   validation.Field,
		}
	case errors.Is(err, userconfig.ErrConflict),
		errors.Is(err, userconfig.ErrRevisionExhausted),
		errors.Is(err, plugins.ErrConfigRevisionConflict),
		errors.Is(err, plugins.ErrConfigRevisionRegression):
		return Problem{Code: ProblemConflict, Message: "revision conflict", CurrentRevision: currentRevision}
	case errors.Is(err, plugins.ErrUnknownPlugin):
		return Problem{Code: ProblemNotFound, Message: "plugin not found"}
	case errors.Is(err, userconfig.ErrUnsupportedPlatform):
		return Problem{Code: ProblemUnsupportedPlatform, Message: "platform is not supported"}
	case errors.Is(err, context.DeadlineExceeded):
		return Problem{Code: ProblemTimeout, Message: "operation timed out"}
	case errors.Is(err, application.ErrInvalidLifecycle),
		errors.Is(err, plugins.ErrManagerNotStarted),
		errors.Is(err, plugins.ErrManagerClosed),
		errors.Is(err, plugins.ErrInvalidState):
		return Problem{Code: ProblemUnavailable, Message: "operation is unavailable"}
	default:
		return Problem{Code: ProblemInternal, Message: "internal operation failed"}
	}
}
