package plugins

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/wzhqwq/vrcft-go/pkg/pluginapi"
	"github.com/wzhqwq/vrcft-go/pkg/protocol"
)

const sessionTokenSize = 32

type handshakeResult struct {
	Descriptor pluginapi.Descriptor
}

// hostHandshake establishes the only legal pre-runtime message sequence:
// Hello, Initialize, then Ready.
func hostHandshake(
	ctx context.Context,
	conn protocol.Conn,
	manifest Manifest,
	token string,
	startup pluginapi.Startup,
) (handshakeResult, error) {
	if ctx == nil || conn == nil {
		return handshakeResult{}, ErrProtocolViolation
	}

	expectedToken, validExpectedToken := decodeSessionToken(token)
	message, err := conn.Receive(ctx)
	if err != nil {
		return handshakeResult{}, handshakeConnectionError(ctx, err, token, startup)
	}
	hello, ok := message.Payload.(protocol.Hello)
	if message.Version != protocol.Version || message.Type != protocol.MessageHello || !ok {
		return handshakeResult{}, ErrProtocolViolation
	}

	providedToken, validProvidedToken := decodeSessionToken(hello.Token)
	if !validExpectedToken || !validProvidedToken || subtle.ConstantTimeCompare(expectedToken, providedToken) != 1 {
		return handshakeResult{}, ErrAuthenticationFailed
	}
	if !helloProtocolCompatible(hello, manifest) {
		return handshakeResult{}, ErrProtocolIncompatible
	}
	if err := hello.Descriptor.Validate(); err != nil || !descriptorMatchesManifest(hello.Descriptor, manifest) {
		return handshakeResult{}, ErrDescriptorMismatch
	}

	initialize, err := protocol.NewMessage(protocol.Initialize{Startup: cloneStartup(startup)})
	if err != nil {
		return handshakeResult{}, ErrProtocolViolation
	}
	if err := conn.Send(ctx, initialize); err != nil {
		return handshakeResult{}, handshakeConnectionError(ctx, err, token, startup)
	}

	message, err = conn.Receive(ctx)
	if err != nil {
		return handshakeResult{}, handshakeConnectionError(ctx, err, token, startup)
	}
	if message.Version != protocol.Version || message.Type != protocol.MessageReady {
		return handshakeResult{}, ErrProtocolViolation
	}
	if _, ok := message.Payload.(protocol.Ready); !ok {
		return handshakeResult{}, ErrProtocolViolation
	}
	return handshakeResult{Descriptor: hello.Descriptor}, nil
}

func decodeSessionToken(token string) ([]byte, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != sessionTokenSize {
		return nil, false
	}
	return decoded, true
}

func helloProtocolCompatible(hello protocol.Hello, manifest Manifest) bool {
	if hello.ProtocolMin > hello.ProtocolMax || hello.ProtocolMin > protocol.Version || hello.ProtocolMax < protocol.Version {
		return false
	}
	if manifest.ProtocolMin > manifest.ProtocolMax || manifest.ProtocolMin > protocol.Version || manifest.ProtocolMax < protocol.Version {
		return false
	}
	return hello.ProtocolMin <= manifest.ProtocolMax && manifest.ProtocolMin <= hello.ProtocolMax
}

func descriptorMatchesManifest(descriptor pluginapi.Descriptor, manifest Manifest) bool {
	return descriptor.ID == manifest.ID &&
		descriptor.Version == manifest.Version &&
		descriptor.Capabilities == manifest.Capabilities
}

func cloneStartup(startup pluginapi.Startup) pluginapi.Startup {
	startup.Config = startup.Config.Clone()
	return startup
}

func handshakeConnectionError(ctx context.Context, err error, token string, startup pluginapi.Startup) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.Join(ErrHandshakeTimeout, context.DeadlineExceeded)
	}
	if !handshakeErrorIsSafe(err, token, startup.Config.Data) {
		return ErrProtocolViolation
	}
	return errors.Join(ErrProtocolViolation, err)
}

func handshakeErrorIsSafe(err error, token string, config []byte) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return (token == "" || !strings.Contains(message, token)) &&
		(len(config) == 0 || !strings.Contains(message, string(config)))
}
