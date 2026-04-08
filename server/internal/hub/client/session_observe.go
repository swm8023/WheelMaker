package client

import (
	"sync"
	"time"
)

var (
	promptWarnAfter       = 60 * time.Second
	promptErrorAfter      = 180 * time.Second
	promptObserveInterval = 1 * time.Second
	timeoutNotifyCooldown = 60 * time.Second
)

type promptObserveEvents struct {
	WarnFirstWait  bool
	ErrorFirstWait bool
	WarnSilence    bool
	ErrorSilence   bool
}

type promptObserveState struct {
	startAt      time.Time
	lastUpdateAt time.Time

	sawFirstUpdate bool

	warnedFirstWait  bool
	erroredFirstWait bool
	warnedSilence    bool
	erroredSilence   bool
}

func newPromptObserveState(now time.Time) *promptObserveState {
	return &promptObserveState{
		startAt:      now,
		lastUpdateAt: now,
	}
}

func (s *promptObserveState) MarkActivity(now time.Time, hasUpdate bool) {
	s.lastUpdateAt = now
	if !hasUpdate {
		return
	}
	s.sawFirstUpdate = true
	s.warnedSilence = false
	s.erroredSilence = false
}

func (s *promptObserveState) Started() bool {
	return s.sawFirstUpdate
}

func (s *promptObserveState) Eval(now time.Time, streamStarted bool) promptObserveEvents {
	e := promptObserveEvents{}
	if !streamStarted {
		elapsed := now.Sub(s.startAt)
		if elapsed >= promptWarnAfter && !s.warnedFirstWait {
			s.warnedFirstWait = true
			e.WarnFirstWait = true
		}
		if elapsed >= promptErrorAfter && !s.erroredFirstWait {
			s.erroredFirstWait = true
			e.ErrorFirstWait = true
		}
		return e
	}

	idle := now.Sub(s.lastUpdateAt)
	if idle >= promptWarnAfter && !s.warnedSilence {
		s.warnedSilence = true
		e.WarnSilence = true
	}
	if idle >= promptErrorAfter && !s.erroredSilence {
		s.erroredSilence = true
		e.ErrorSilence = true
	}
	return e
}

type timeoutNotifyLimiter struct {
	mu        sync.Mutex
	cooldown  time.Duration
	lastByKey map[string]time.Time
}

func newTimeoutNotifyLimiter(cooldown time.Duration) *timeoutNotifyLimiter {
	return &timeoutNotifyLimiter{
		cooldown:  cooldown,
		lastByKey: map[string]time.Time{},
	}
}

func (l *timeoutNotifyLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	last, ok := l.lastByKey[key]
	if ok && now.Sub(last) < l.cooldown {
		return false
	}
	l.lastByKey[key] = now
	return true
}
