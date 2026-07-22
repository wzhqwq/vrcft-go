package protocol

import "context"

type Conn interface {
	Send(context.Context, Message) error
	Receive(context.Context) (Message, error)
	Close() error
}
