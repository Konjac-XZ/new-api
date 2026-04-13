package model

import "github.com/QuantumNous/new-api/common"

type BreakerPenaltyTrace struct {
	Id                   int     `json:"id" gorm:"primaryKey;autoIncrement"`
	ChannelId            int     `json:"channel_id" gorm:"not null;index:idx_breaker_penalty_trace_channel_created,priority:1"`
	CreatedAt            int64   `json:"created_at" gorm:"bigint;not null;index:idx_breaker_penalty_trace_channel_created,priority:2;index:idx_breaker_penalty_trace_created"`
	EventType            string  `json:"event_type" gorm:"type:varchar(32);not null;index"`
	FailureKind          string  `json:"failure_kind" gorm:"type:varchar(64);not null;index"`
	WasInProbation       bool    `json:"was_in_probation"`
	WasAwaitingProbe     bool    `json:"was_awaiting_probe"`
	ForceCooldown        bool    `json:"force_cooldown"`
	TriggeredCooldown    bool    `json:"triggered_cooldown"`
	CooldownAtBefore     int64   `json:"cooldown_at_before" gorm:"bigint;default:0"`
	CooldownAtAfter      int64   `json:"cooldown_at_after" gorm:"bigint;default:0"`
	PressureBefore       float64 `json:"pressure_before"`
	PressureAfter        float64 `json:"pressure_after"`
	FailStreakBefore     int     `json:"fail_streak_before"`
	FailStreakAfter      int     `json:"fail_streak_after"`
	TripCountBefore      int     `json:"trip_count_before"`
	TripCountAfter       int     `json:"trip_count_after"`
	HPBefore             float64 `json:"hp_before"`
	HPDamage             float64 `json:"hp_damage"`
	HPAfter              float64 `json:"hp_after"`
	BaseCooldownSeconds  int64   `json:"base_cooldown_seconds" gorm:"bigint;default:0"`
	CooldownMultiplier   float64 `json:"cooldown_multiplier"`
	ChronicFloorSeconds  int64   `json:"chronic_floor_seconds" gorm:"bigint;default:0"`
	FinalCooldownSeconds int64   `json:"final_cooldown_seconds" gorm:"bigint;default:0"`
	CalculationInputs    string  `json:"calculation_inputs" gorm:"type:text"`
	CalculationSteps     string  `json:"calculation_steps" gorm:"type:text"`
	CalculationResult    string  `json:"calculation_result" gorm:"type:text"`
}

func GetBreakerPenaltyTracePageByChannelID(channelID int, page int, pageSize int) ([]*BreakerPenaltyTrace, int64, error) {
	if channelID <= 0 {
		return []*BreakerPenaltyTrace{}, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	if pageSize > 50 {
		pageSize = 50
	}

	query := DB.Model(&BreakerPenaltyTrace{}).Where("channel_id = ?", channelID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	traces := make([]*BreakerPenaltyTrace, 0, pageSize)
	if err := query.Order("created_at DESC").Order("id DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&traces).Error; err != nil {
		return nil, 0, err
	}

	return traces, total, nil
}

func CleanupBreakerPenaltyTraceRecords(olderThanSeconds int64, limit int) (int64, error) {
	if olderThanSeconds <= 0 {
		olderThanSeconds = 90 * 24 * 3600
	}
	if limit <= 0 {
		limit = 1000
	}
	cutoff := common.GetTimestamp() - olderThanSeconds
	var total int64

	for {
		var ids []int
		if err := DB.Model(&BreakerPenaltyTrace{}).
			Where("created_at < ?", cutoff).
			Order("id ASC").
			Limit(limit).
			Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			break
		}

		result := DB.Where("id IN ?", ids).Delete(&BreakerPenaltyTrace{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if len(ids) < limit {
			break
		}
	}

	return total, nil
}
