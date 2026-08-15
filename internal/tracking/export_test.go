package tracking

// NewServiceWithClockForTest exposes the injectable-clock constructor to
// external integration tests without adding a production API.
func NewServiceWithClockForTest(now func() int64) Service {
	return newServiceWithClock(now)
}
