package tracking

import "context"

func offerLatest[T any](channel chan T, value T) {
	select {
	case channel <- value:
		return
	default:
	}
	select {
	case <-channel:
	default:
	}
	select {
	case channel <- value:
	default:
	}
}

func (s *service) SubscribeMerged(ctx context.Context) <-chan MergedFrame {
	updates := make(chan MergedFrame, 1)
	if ctx.Err() != nil {
		close(updates)
		return updates
	}

	s.mu.Lock()
	s.mergedSubscribers[updates] = struct{}{}
	if s.hasLatest {
		offerLatest(updates, s.latestMerged)
	}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if _, ok := s.mergedSubscribers[updates]; ok {
			delete(s.mergedSubscribers, updates)
			close(updates)
		}
		s.mu.Unlock()
	}()
	return updates
}

func (s *service) SubscribeSummary(ctx context.Context) <-chan Summary {
	updates := make(chan Summary, 1)
	if ctx.Err() != nil {
		close(updates)
		return updates
	}

	s.mu.Lock()
	s.summarySubscribers[updates] = struct{}{}
	offerLatest(updates, s.currentSummaryLocked())
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if _, ok := s.summarySubscribers[updates]; ok {
			delete(s.summarySubscribers, updates)
			close(updates)
		}
		s.mu.Unlock()
	}()
	return updates
}

func (s *service) publishMergedLocked() {
	for subscriber := range s.mergedSubscribers {
		offerLatest(subscriber, s.latestMerged)
	}
}

func (s *service) publishSummaryLocked() {
	summary := s.currentSummaryLocked()
	for subscriber := range s.summarySubscribers {
		offerLatest(subscriber, summary)
	}
}
