package service

import (
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

var channelBreakerLocks sync.Map
var channelBreakerWorkingState sync.Map

type channelBreakerStateSnapshot struct {
	BreakerPressure       float64
	BreakerUpdatedAt      int64
	BreakerFailStreak     int
	BreakerCooldownAt     int64
	BreakerLastFailure    string
	BreakerHP             float64
	BreakerTripCount      int
	BreakerRecentRequests float64
	BreakerRecentFailures float64
	BreakerRecentTimeouts float64
}

func getChannelBreakerLock(channelID int) *sync.Mutex {
	if lock, ok := channelBreakerLocks.Load(channelID); ok {
		return lock.(*sync.Mutex)
	}
	lock := &sync.Mutex{}
	actual, _ := channelBreakerLocks.LoadOrStore(channelID, lock)
	return actual.(*sync.Mutex)
}

func snapshotChannelBreakerState(channel *model.Channel) channelBreakerStateSnapshot {
	if channel == nil {
		return channelBreakerStateSnapshot{}
	}
	return channelBreakerStateSnapshot{
		BreakerPressure:       channel.BreakerPressure,
		BreakerUpdatedAt:      channel.BreakerUpdatedAt,
		BreakerFailStreak:     channel.BreakerFailStreak,
		BreakerCooldownAt:     channel.BreakerCooldownAt,
		BreakerLastFailure:    channel.BreakerLastFailure,
		BreakerHP:             channel.BreakerHP,
		BreakerTripCount:      channel.BreakerTripCount,
		BreakerRecentRequests: channel.BreakerRecentRequests,
		BreakerRecentFailures: channel.BreakerRecentFailures,
		BreakerRecentTimeouts: channel.BreakerRecentTimeouts,
	}
}

func applyChannelBreakerState(channel *model.Channel, snapshot channelBreakerStateSnapshot) {
	if channel == nil {
		return
	}
	channel.BreakerPressure = snapshot.BreakerPressure
	channel.BreakerUpdatedAt = snapshot.BreakerUpdatedAt
	channel.BreakerFailStreak = snapshot.BreakerFailStreak
	channel.BreakerCooldownAt = snapshot.BreakerCooldownAt
	channel.BreakerLastFailure = snapshot.BreakerLastFailure
	channel.BreakerHP = snapshot.BreakerHP
	channel.BreakerTripCount = snapshot.BreakerTripCount
	channel.BreakerRecentRequests = snapshot.BreakerRecentRequests
	channel.BreakerRecentFailures = snapshot.BreakerRecentFailures
	channel.BreakerRecentTimeouts = snapshot.BreakerRecentTimeouts
}

func loadChannelBreakerWorkingCopy(channel *model.Channel) *model.Channel {
	if channel == nil {
		return nil
	}
	current := *channel
	if cached, ok := channelBreakerWorkingState.Load(channel.Id); ok {
		applyChannelBreakerState(&current, cached.(channelBreakerStateSnapshot))
	}
	return &current
}

func persistChannelBreakerState(current *model.Channel) error {
	if current == nil {
		return errors.New("channel breaker state is nil")
	}
	if err := model.UpdateChannelBreakerState(current); err != nil {
		return err
	}
	channelBreakerWorkingState.Store(current.Id, snapshotChannelBreakerState(current))
	return nil
}

func mutateChannelBreakerState(channel *model.Channel, mutate func(current *model.Channel, now time.Time) bool) (bool, error) {
	if channel == nil || mutate == nil {
		return false, nil
	}

	lock := getChannelBreakerLock(channel.Id)
	lock.Lock()
	defer lock.Unlock()

	current := loadChannelBreakerWorkingCopy(channel)
	if !current.IsDynamicCircuitBreakerEnabled() {
		return false, nil
	}

	now := time.Now()
	if !mutate(current, now) {
		return false, nil
	}

	if err := persistChannelBreakerState(current); err != nil {
		return false, err
	}
	return true, nil
}

func clearChannelBreakerWorkingState(channelID int) {
	channelBreakerWorkingState.Delete(channelID)
	channelBreakerLocks.Delete(channelID)
}
