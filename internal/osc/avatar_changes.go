package osc

import (
	"context"
	"math"
	"sync"
)

type AvatarChange struct {
	Revision uint64
	AvatarID string
}

type avatarChangeMailbox struct {
	mu          sync.Mutex
	revision    uint64
	closed      bool
	subscribers map[chan AvatarChange]struct{}
}

func newAvatarChangeMailbox() avatarChangeMailbox {
	return avatarChangeMailbox{subscribers: make(map[chan AvatarChange]struct{})}
}

func (mailbox *avatarChangeMailbox) subscribe(ctx context.Context) <-chan AvatarChange {
	if ctx == nil {
		ctx = context.Background()
	}
	changes := make(chan AvatarChange, 1)
	mailbox.mu.Lock()
	if mailbox.closed {
		close(changes)
		mailbox.mu.Unlock()
		return changes
	}
	mailbox.subscribers[changes] = struct{}{}
	mailbox.mu.Unlock()

	if done := ctx.Done(); done != nil {
		go func() {
			<-done
			mailbox.mu.Lock()
			if _, ok := mailbox.subscribers[changes]; ok {
				delete(mailbox.subscribers, changes)
				close(changes)
			}
			mailbox.mu.Unlock()
		}()
	}
	return changes
}

func (mailbox *avatarChangeMailbox) publish(avatarID string) AvatarChange {
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()

	if mailbox.revision < math.MaxUint64 {
		mailbox.revision++
	}
	change := AvatarChange{Revision: mailbox.revision, AvatarID: avatarID}
	if mailbox.closed {
		return change
	}
	for subscriber := range mailbox.subscribers {
		select {
		case subscriber <- change:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- change:
			default:
			}
		}
	}
	return change
}

func (mailbox *avatarChangeMailbox) close() {
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()
	if mailbox.closed {
		return
	}
	mailbox.closed = true
	for subscriber := range mailbox.subscribers {
		delete(mailbox.subscribers, subscriber)
		close(subscriber)
	}
}
