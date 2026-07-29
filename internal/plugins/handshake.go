package plugins

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"

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
		return handshakeResult{}, handshakeConnectionError(ctx, err)
	}
	hello, ok := message.Payload.(protocol.Hello)
	if message.Version != protocol.Version || message.Type != protocol.MessageHello || !ok {
		return handshakeResult{}, ErrProtocolViolation
	}

	providedToken, validProvidedToken := decodeSessionToken(hello.Token)
	tokensMatch := subtle.ConstantTimeCompare(expectedToken[:], providedToken[:])
	if validExpectedToken&validProvidedToken&tokensMatch != 1 {
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
		return handshakeResult{}, handshakeConnectionError(ctx, err)
	}

	message, err = conn.Receive(ctx)
	if err != nil {
		return handshakeResult{}, handshakeConnectionError(ctx, err)
	}
	if message.Version != protocol.Version || message.Type != protocol.MessageReady {
		return handshakeResult{}, ErrProtocolViolation
	}
	if _, ok := message.Payload.(protocol.Ready); !ok {
		return handshakeResult{}, ErrProtocolViolation
	}
	return handshakeResult{Descriptor: hello.Descriptor}, nil
}

func decodeSessionToken(token string) ([sessionTokenSize]byte, int) {
	var decodedToken [sessionTokenSize]byte
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	copy(decodedToken[:], decoded)

	valid := subtle.ConstantTimeEq(int32(len(decoded)), sessionTokenSize)
	canonical := base64.RawURLEncoding.EncodeToString(decodedToken[:])
	valid &= subtle.ConstantTimeCompare([]byte(token), []byte(canonical))
	if err != nil {
		valid = 0
	}
	return decodedToken, valid
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

func handshakeConnectionError(ctx context.Context, err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.Join(ErrHandshakeTimeout, context.DeadlineExceeded)
	}
	return opaqueHandshakeCause{cause: err}
}

type opaqueHandshakeCause struct{ cause error }

func (e opaqueHandshakeCause) Error() string { return "plugins: handshake connection failure" }

func (e opaqueHandshakeCause) Is(target error) bool { return errors.Is(e.cause, target) }
