package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type channelBreakerDetailResponse struct {
	Id           int                 `json:"id"`
	Name         string              `json:"name"`
	BreakerState ChannelBreakerState `json:"breaker_state"`
	TracePage    struct {
		Items []struct {
			Id                int            `json:"id"`
			EventType         string         `json:"event_type"`
			FailureKind       string         `json:"failure_kind"`
			CalculationInputs map[string]any `json:"calculation_inputs"`
			CalculationSteps  []string       `json:"calculation_steps"`
			CalculationResult map[string]any `json:"calculation_result"`
		} `json:"items"`
		Total int64 `json:"total"`
	} `json:"trace_page"`
}

type channelListResponse struct {
	Items []struct {
		Id           int                 `json:"id"`
		Name         string              `json:"name"`
		BreakerState ChannelBreakerState `json:"breaker_state"`
	} `json:"items"`
	Total int64 `json:"total"`
}

func setupChannelBreakerDetailTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	if err := db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBreakerState{}, &model.ChannelTestConfig{}, &model.BreakerPenaltyTrace{}); err != nil {
		t.Fatalf("failed to migrate breaker detail tables: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestGetChannelBreakerDetailReturnsTraceHistory(t *testing.T) {
	db := setupChannelBreakerDetailTestDB(t)

	autoBan := 1
	settingsBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("failed to marshal channel settings: %v", err)
	}
	settings := string(settingsBytes)
	now := time.Now().Unix()
	channel := &model.Channel{
		Name:               "breaker-detail-channel",
		Key:                "sk-breaker-detail",
		Status:             common.ChannelStatusEnabled,
		Group:              "default",
		Models:             "gpt-4o-mini",
		AutoBan:            &autoBan,
		Setting:            &settings,
		BreakerPressure:    2.75,
		BreakerFailStreak:  3,
		BreakerCooldownAt:  now + 180,
		BreakerUpdatedAt:   now,
		BreakerLastFailure: "first_token_timeout",
		BreakerHP:          0,
		BreakerTripCount:   2,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	trace := &model.BreakerPenaltyTrace{
		ChannelId:            channel.Id,
		CreatedAt:            now,
		EventType:            "relay_failure",
		FailureKind:          "first_token_timeout",
		TriggeredCooldown:    true,
		PressureBefore:       1.2,
		PressureAfter:        2.75,
		FailStreakBefore:     2,
		FailStreakAfter:      3,
		TripCountBefore:      1,
		TripCountAfter:       2,
		HPBefore:             2.5,
		HPDamage:             2.5,
		HPAfter:              0,
		BaseCooldownSeconds:  90,
		CooldownMultiplier:   2.1,
		ChronicFloorSeconds:  120,
		FinalCooldownSeconds: 189,
		CalculationInputs:    `{"pressure_before":1.2,"final_cooldown_seconds":189}`,
		CalculationSteps:     "Step 1: HP_after = HP_before - damage\nResult: cooldown_at = now + 189s",
		CalculationResult:    `{"triggered_cooldown":true,"final_cooldown_seconds":189}`,
	}
	if err := db.Create(trace).Error; err != nil {
		t.Fatalf("failed to create breaker penalty trace: %v", err)
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/channel/1/breaker/detail", nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	GetChannelBreakerDetail(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail channelBreakerDetailResponse
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode breaker detail response: %v", err)
	}

	if detail.Id != channel.Id {
		t.Fatalf("expected channel id=%d, got %d", channel.Id, detail.Id)
	}
	if !detail.BreakerState.DynamicEnabled {
		t.Fatal("expected breaker state to report dynamic breaker enabled")
	}
	if detail.TracePage.Total != 1 || len(detail.TracePage.Items) != 1 {
		t.Fatalf("expected exactly one trace item, got total=%d len=%d", detail.TracePage.Total, len(detail.TracePage.Items))
	}
	if detail.TracePage.Items[0].EventType != "relay_failure" {
		t.Fatalf("expected event_type relay_failure, got %q", detail.TracePage.Items[0].EventType)
	}
	if len(detail.TracePage.Items[0].CalculationSteps) != 2 {
		t.Fatalf("expected two calculation steps, got %d", len(detail.TracePage.Items[0].CalculationSteps))
	}
	if detail.TracePage.Items[0].CalculationInputs["pressure_before"] == nil {
		t.Fatal("expected decoded calculation input payload")
	}
	if detail.TracePage.Items[0].CalculationResult["triggered_cooldown"] == nil {
		t.Fatal("expected decoded calculation result payload")
	}
}

func TestGetAllChannelsLoadsExternalBreakerConfig(t *testing.T) {
	setupChannelBreakerDetailTestDB(t)

	autoBan := 1
	settingsBytes, err := common.Marshal(dto.ChannelSettings{DynamicCircuitBreaker: true})
	if err != nil {
		t.Fatalf("failed to marshal channel settings: %v", err)
	}
	settings := string(settingsBytes)
	channel := &model.Channel{
		Name:    "breaker-list-channel",
		Key:     "sk-breaker-list",
		Status:  common.ChannelStatusEnabled,
		Group:   "default",
		Models:  "gpt-4o-mini",
		AutoBan: &autoBan,
		Setting: &settings,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/channel/?p=1&page_size=20", nil, 1)
	GetAllChannels(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var list channelListResponse
	if err := common.Unmarshal(response.Data, &list); err != nil {
		t.Fatalf("failed to decode channel list response: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("expected one list item, got total=%d len=%d", list.Total, len(list.Items))
	}
	if list.Items[0].Id != channel.Id {
		t.Fatalf("expected channel id=%d, got %d", channel.Id, list.Items[0].Id)
	}
	if !list.Items[0].BreakerState.DynamicEnabled {
		t.Fatal("expected list breaker state to report dynamic breaker enabled")
	}
}

func TestUpdateChannelPreservesExternalConfigWhenRequestOmitsFields(t *testing.T) {
	setupChannelBreakerDetailTestDB(t)

	autoBan := 1
	priority := int64(10)
	weight := uint(100)
	coeff := 2.5
	maxLatency := 6
	interval := 15
	testCase := "ping"
	expectedAnswer := "pong"
	channel := &model.Channel{
		Name:                     "preserve-external-channel",
		Key:                      "sk-preserve",
		Status:                   common.ChannelStatusEnabled,
		Group:                    "default",
		Models:                   "gpt-4o-mini",
		Priority:                 &priority,
		Weight:                   &weight,
		AutoBan:                  &autoBan,
		DynamicCircuitBreaker:    true,
		ToleranceCoefficient:     &coeff,
		MaxRetryAttempts:         4,
		TreatEmptyReplyAsFailure: true,
		TestCase:                 &testCase,
		ExpectedAnswer:           &expectedAnswer,
		MaxFirstTokenLatency:     &maxLatency,
		ScheduledTestInterval:    &interval,
	}
	if err := channel.Insert(); err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	payload := gin.H{
		"id":                  channel.Id,
		"name":                "preserve-external-channel-updated",
		"type":                channel.Type,
		"key":                 "",
		"status":              common.ChannelStatusEnabled,
		"group":               channel.Group,
		"models":              channel.Models,
		"priority":            priority,
		"weight":              weight,
		"auto_ban":            autoBan,
		"base_url":            "",
		"openai_organization": "",
		"test_model":          "",
		"tag":                 "",
		"remark":              "",
		"model_mapping":       "",
		"status_code_mapping": "",
		"param_override":      "",
		"header_override":     "",
		"setting":             "{}",
		"settings":            "{}",
		"other":               "",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/channel/", payload, 1)

	UpdateChannel(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	reloaded, err := model.GetChannelById(channel.Id, true)
	if err != nil {
		t.Fatalf("failed to reload channel: %v", err)
	}
	if !reloaded.DynamicCircuitBreaker {
		t.Fatal("expected dynamic breaker config to be preserved")
	}
	if reloaded.ToleranceCoefficient == nil || *reloaded.ToleranceCoefficient != coeff {
		t.Fatalf("expected tolerance coefficient %.1f, got %#v", coeff, reloaded.ToleranceCoefficient)
	}
	if reloaded.GetMaxRetryAttempts() != 4 {
		t.Fatalf("expected max retry attempts 4, got %d", reloaded.GetMaxRetryAttempts())
	}
	if !reloaded.TreatEmptyReplyAsFailure {
		t.Fatal("expected empty-reply failure config to be preserved")
	}
	if reloaded.TestCase == nil || *reloaded.TestCase != testCase {
		t.Fatalf("expected test case %q, got %#v", testCase, reloaded.TestCase)
	}
	if reloaded.ExpectedAnswer == nil || *reloaded.ExpectedAnswer != expectedAnswer {
		t.Fatalf("expected expected answer %q, got %#v", expectedAnswer, reloaded.ExpectedAnswer)
	}
	if reloaded.GetMaxFirstTokenLatency() != maxLatency {
		t.Fatalf("expected max first token latency %d, got %d", maxLatency, reloaded.GetMaxFirstTokenLatency())
	}
	if reloaded.GetScheduledTestInterval() != interval {
		t.Fatalf("expected scheduled test interval %d, got %d", interval, reloaded.GetScheduledTestInterval())
	}
}
