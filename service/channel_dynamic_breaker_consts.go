package service

import "time"

type channelFailureKind string

type channelSuccessLatencyClass int

const (
	channelFailureKindGeneric           channelFailureKind = "generic"
	channelFailureKindImmediateFailure  channelFailureKind = "immediate_failure"
	channelFailureKindFirstTokenTimeout channelFailureKind = "first_token_timeout"
	channelFailureKindMidStreamFailure  channelFailureKind = "mid_stream_failure"
	channelFailureKindOverloaded        channelFailureKind = "overloaded"
	channelFailureKindEmptyReply        channelFailureKind = "empty_reply"

	channelSuccessLatencyNormal channelSuccessLatencyClass = iota
	channelSuccessLatencyFast
	channelSuccessLatencyNearTimeout
)

// Pressure system constants (unchanged — controls cooldown duration)
const (
	breakerDecayWindow                    = 4 * time.Hour
	breakerMaxCooldown                    = 90 * time.Minute
	breakerMinimumCooldown                = 2 * time.Minute
	breakerNormalRecoveryFactor           = 0.7
	breakerProbationRecoveryFactor        = 0.45
	breakerSlowSuccessPressure            = 0.35
	breakerProbationPenalty               = 0.75
	breakerProbationSilentTimeoutPenalty  = 3.0
	breakerAwaitingProbePenalty           = 4.0
	breakerProbeTestPenalty               = 2.5
	breakerProbeObservationPenalty        = 5.0
	breakerMinPressure                    = 0.05
	breakerMaxPressureContribution        = 100.0
	breakerSlowSuccessThreshold           = 6 * time.Second
	breakerFastSuccessPressureFactor      = 0.35
	breakerPressureCooldownWeight         = 0.7
	breakerFailStreakCooldownWeight       = 1.35
	breakerFailStreakCooldownExponent     = 0.9
	breakerFailStreakCooldownCap          = 40
	breakerTripCooldownWeight             = 12.0
	breakerTripCooldownStart              = 1
	breakerTripCooldownCap                = 60
	breakerFailureRateCooldownThreshold   = 0.65
	breakerFailureRateCooldownWeight      = 25.0
	breakerFailureRateCooldownExponent    = 1.35
	breakerTimeoutRateCooldownThreshold   = 0.35
	breakerTimeoutRateCooldownWeight      = 12.0
	breakerTimeoutRateCooldownExponent    = 1.25
	breakerCooldownRateConfidenceRequests = 10.0
	breakerShortTermPenaltyMinFactor      = 0.08
	breakerShortTermPressureMinFactor     = 0.35
	breakerShortTermStreakScale           = 6.0
	breakerShortTermStreakExponent        = 1.1
	breakerShortTermRateExponent          = 1.35
	breakerShortTermHistoryExponent       = 1.35
	breakerChronicTripFloorStart          = 2
	breakerChronicTripFloorWeight         = 25.0
	breakerChronicFailureFloorThreshold   = 0.8
	breakerChronicFailureFloorWeight      = 100.0
	breakerChronicFailureFloorExponent    = 1.4
	breakerChronicStreakFloorStart        = 10
	breakerChronicStreakFloorWeight       = 6.5
	breakerChronicStreakFloorExponent     = 0.75
)

// HP system constants — controls whether cooldown triggers
const (
	hpBase                               = 10.0      // base max HP before coefficient
	hpMinCoefficient                     = 0.1       // minimum tolerance coefficient
	hpMaxCoefficient                     = 10.0      // maximum tolerance coefficient
	hpDefaultCoefficient                 = 1.0       // default when not configured
	hpMinimum                            = 1.0       // minimum maxHP floor
	hpPassiveRecoveryPerHour             = 0.5       // HP recovered per hour passively
	hpSuccessRecovery                    = 1.0       // HP recovered per successful request
	hpFastSuccessRecoveryBonus           = 0.5       // additional HP for fast streaming success
	hpProbationSuccessRecovery           = 0.8       // HP recovered per success during observation
	hpProbationDamageMultiplier          = 2.5       // damage multiplier during observation
	hpProbationSilentTimeoutDamageMultiplier = 2.0   // implicit timeout during observation is treated as a severe signal
	hpAwaitingProbeDamageMultiplier      = 2.0       // damage multiplier when awaiting probe
	hpProbeSuccessRefillFraction         = 0.35      // probe success: refill to 35% of maxHP (not full)
	hpFastProbeSuccessRefillFraction     = 0.45      // fast organic recovery: refill a bit more aggressively
	hpProbationSuccessRefillFraction     = 0.70      // probation success: refill to 70% of maxHP
	hpFastProbationSuccessRefillFraction = 0.85      // fast probation success: refill deeper to restore capacity sooner
	hpEWMADecayWindow                    = 6 * time.Hour // EWMA decay time constant
	hpEWMAMinValue                       = 0.01      // minimum EWMA value before zeroing
	hpSuccessRewardConfidence            = 16.0      // recent successful requests needed to unlock the full bonus
	hpSuccessRewardMaxBonus              = 1.0       // maximum additive multiplier for sustained success
	hpQuickHonestFailureDamageFactor     = 0.6       // quick explicit streaming failure should be cheap
	hpDelayedFailureDamageFactor         = 1.15      // delayed no-response failure is worse than an honest quick error
	hpTimeoutFailureDamageFactor         = 1.75      // first-token timeout is the harshest signal
)
