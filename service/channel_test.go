package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannel_AbnormalStatusFollowsRetryRule(t *testing.T) {
	origAutoDisable := common.AutomaticDisableChannelEnabled
	origRetryRanges := operation_setting.AutomaticRetryStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = origAutoDisable
		operation_setting.AutomaticRetryStatusCodeRanges = origRetryRanges
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 500, End: 599}}

	err := types.NewErrorWithStatusCode(errors.New("upstream status error"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)
	require.True(t, ShouldDisableChannel(0, err))
}

func TestShouldDisableChannel_NormalStatusNotDisabled(t *testing.T) {
	origAutoDisable := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = origAutoDisable
	})

	common.AutomaticDisableChannelEnabled = true

	err := types.NewErrorWithStatusCode(errors.New("ok"), types.ErrorCodeBadResponseStatusCode, http.StatusOK)
	require.False(t, ShouldDisableChannel(0, err))
}

func TestShouldDisableChannel_SkipRetryErrorNotDisabledByStatus(t *testing.T) {
	origAutoDisable := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = origAutoDisable
	})

	common.AutomaticDisableChannelEnabled = true

	err := types.NewErrorWithStatusCode(
		errors.New("upstream status error"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
		types.ErrOptionWithSkipRetry(),
	)
	require.False(t, ShouldDisableChannel(0, err))
}
