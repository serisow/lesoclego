// pipeline/time_provider.go
package pipeline

import "time"

type TimeProvider interface {
    Now() time.Time
}

type realTimeProvider struct{}

func (rtp *realTimeProvider) Now() time.Time {
    return time.Now()
}

var timeProvider TimeProvider = &realTimeProvider{}
