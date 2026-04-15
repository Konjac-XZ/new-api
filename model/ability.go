package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getPriority(group string, model string, retry int) (int, error) {

	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true).
		Order("priority DESC").              // 按优先级降序排序
		Pluck("priority", &priorities).Error // Pluck用于将查询的结果直接扫描到一个切片中

	if err != nil {
		// 处理错误
		return 0, err
	}

	if len(priorities) == 0 {
		// 如果没有查询到优先级，则返回错误
		return 0, errors.New("数据库一致性被破坏")
	}

	// 确定要使用的优先级
	var priorityToUse int
	if retry >= len(priorities) {
		// 如果重试次数大于优先级数，则使用最小的优先级
		priorityToUse = priorities[len(priorities)-1]
	} else {
		priorityToUse = priorities[retry]
	}
	return priorityToUse, nil
}

func getChannelQuery(group string, model string, retry int) (*gorm.DB, error) {
	maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true)
	channelQuery := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = (?)", group, model, true, maxPrioritySubQuery)
	if retry != 0 {
		priority, err := getPriority(group, model, retry)
		if err != nil {
			return nil, err
		} else {
			channelQuery = DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?", group, model, true, priority)
		}
	}

	return channelQuery, nil
}

func GetChannel(group string, model string, retry int) (*Channel, error) {
	var abilities []Ability

	var err error = nil
	channelQuery, err := getChannelQuery(group, model, retry)
	if err != nil {
		return nil, err
	}
	if common.UsingSQLite || common.UsingPostgreSQL {
		err = channelQuery.Order("weight DESC").Find(&abilities).Error
	} else {
		err = channelQuery.Order("weight DESC").Find(&abilities).Error
	}
	if err != nil {
		return nil, err
	}
	channel := Channel{}
	if len(abilities) > 0 {
		// Randomly choose one
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		// Randomly choose one
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			//log.Printf("weight: %d, ability weight: %d", weight, *ability_.Weight)
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err = DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

// GetChannelExclude selects a channel with the highest available priority,
// excluding any channels listed in `exclude`. It retries within the same
// priority (weighted) until all channels of that priority are exhausted,
// then falls back to the next lower priority.
//
// Soft priority fallback: when more than half the channels at a priority tier
// are excluded, channels from the next lower priority are blended in with
// halved effective weights.
func GetChannelExclude(group string, model string, exclude map[int]bool) (*Channel, error) {
	// Collect distinct priorities in descending order
	var priorities []int
	err := DB.Model(&Ability{}).
		Select("DISTINCT(priority)").
		Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true).
		Order("priority DESC").
		Pluck("priority", &priorities).Error
	if err != nil {
		return nil, err
	}
	if len(priorities) == 0 {
		return nil, errors.New("数据库一致性被破坏")
	}

	type candidate struct {
		channelId int
		weight    int
	}

	// Iterate priorities from highest to lowest
	for idx, p := range priorities {
		var abilities []Ability
		q := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?", group, model, true, p)
		if common.UsingSQLite || common.UsingPostgreSQL {
			err = q.Order("weight DESC").Find(&abilities).Error
		} else {
			err = q.Order("weight DESC").Find(&abilities).Error
		}
		if err != nil {
			return nil, err
		}

		if len(abilities) == 0 {
			continue
		}

		channelIDs := make([]int, 0, len(abilities))
		for _, a := range abilities {
			channelIDs = append(channelIDs, a.ChannelId)
		}
		channelByID, mapErr := getChannelMapByIDs(channelIDs)
		if mapErr != nil {
			return nil, mapErr
		}

		// Filter out excluded channels at this priority
		totalAtPriority := len(abilities)
		candidates := make([]candidate, 0, len(abilities))
		sumWeight := 0
		for _, a := range abilities {
			if exclude != nil && exclude[a.ChannelId] {
				continue
			}
			w := int(a.Weight)
			if ch, ok := channelByID[a.ChannelId]; ok {
				w = ch.GetEffectiveRoutingWeight(w)
			}
			candidates = append(candidates, candidate{channelId: a.ChannelId, weight: w})
			sumWeight += w
		}
		if len(candidates) == 0 {
			// all channels at this priority have been used
			continue
		}

		// Soft priority fallback: when more than half the tier is excluded
		// and a lower priority tier exists, blend in next-tier channels
		// with halved weights.
		if len(candidates)*2 < totalAtPriority && idx+1 < len(priorities) {
			nextP := priorities[idx+1]
			var nextAbilities []Ability
			nq := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ? and priority = ?", group, model, true, nextP)
			if common.UsingSQLite || common.UsingPostgreSQL {
				err = nq.Order("weight DESC").Find(&nextAbilities).Error
			} else {
				err = nq.Order("weight DESC").Find(&nextAbilities).Error
			}
			if err == nil {
				nextChannelIDs := make([]int, 0, len(nextAbilities))
				for _, a := range nextAbilities {
					nextChannelIDs = append(nextChannelIDs, a.ChannelId)
				}
				nextChannelByID, nextMapErr := getChannelMapByIDs(nextChannelIDs)
				if nextMapErr != nil {
					return nil, nextMapErr
				}
				for _, a := range nextAbilities {
					if exclude != nil && exclude[a.ChannelId] {
						continue
					}
					w := int(a.Weight) / 2
					if ch, ok := nextChannelByID[a.ChannelId]; ok {
						w = ch.GetEffectiveRoutingWeight(w)
					}
					if w < 1 {
						w = 1
					}
					candidates = append(candidates, candidate{channelId: a.ChannelId, weight: w})
					sumWeight += w
				}
			}
		}

		// Smoothing same as memory cache path
		smoothingFactor := 1
		smoothingAdjustment := 0
		if sumWeight == 0 {
			sumWeight = len(candidates) * 100
			smoothingAdjustment = 100
		} else if sumWeight/len(candidates) < 10 {
			smoothingFactor = 100
		}

		totalWeight := sumWeight * smoothingFactor
		w := common.GetRandomInt(totalWeight)
		var chosen candidate
		for _, c := range candidates {
			w -= c.weight*smoothingFactor + smoothingAdjustment
			if w < 0 {
				chosen = c
				break
			}
		}
		if chosen.channelId == 0 {
			// fallback to first
			chosen = candidates[0]
		}

		channel := Channel{}
		err = DB.First(&channel, "id = ?", chosen.channelId).Error
		return &channel, err
	}

	// nothing available
	return nil, nil
}

func getChannelMapByIDs(channelIDs []int) (map[int]*Channel, error) {
	channelByID := make(map[int]*Channel, len(channelIDs))
	if len(channelIDs) == 0 {
		return channelByID, nil
	}

	uniqueIDs := make([]int, 0, len(channelIDs))
	seen := make(map[int]struct{}, len(channelIDs))
	for _, id := range channelIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}

	channels := make([]*Channel, 0, len(uniqueIDs))
	if err := DB.Where("id IN ?", uniqueIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, ch := range channels {
		if ch == nil {
			continue
		}
		channelByID[ch.Id] = ch
	}
	return channelByID, nil
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.IsHardConstraintEnabled(),
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.IsHardConstraintEnabled(),
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, errors.New("已经有一个修复任务在运行中，请稍后再试")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingSQLite {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(fmt.Sprintf("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}
