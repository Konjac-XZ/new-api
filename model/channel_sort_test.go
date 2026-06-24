package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelSortOptionsApplyMultipleRules(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))

	weight10 := uint(10)
	weight20 := uint(20)
	weight30 := uint(30)
	channels := []Channel{
		{Id: 1, Name: "beta", Priority: int64PtrForChannelSortTest(5), Weight: &weight20},
		{Id: 2, Name: "alpha", Priority: int64PtrForChannelSortTest(5), Weight: &weight20},
		{Id: 3, Name: "alpha", Priority: int64PtrForChannelSortTest(5), Weight: &weight20},
		{Id: 4, Name: "delta", Priority: int64PtrForChannelSortTest(8), Weight: &weight10},
		{Id: 5, Name: "gamma", Priority: int64PtrForChannelSortTest(5), Weight: &weight30},
	}
	require.NoError(t, db.Create(&channels).Error)

	options := NewChannelSortOptionsFromRules([]ChannelSortRule{
		{SortBy: "priority", SortOrder: "desc"},
		{SortBy: "weight", SortOrder: "desc"},
		{SortBy: "name", SortOrder: "asc"},
		{SortBy: "id", SortOrder: "desc"},
	}, false)

	var got []Channel
	require.NoError(t, options.Apply(db).Find(&got).Error)

	require.Equal(t, []int{4, 5, 3, 2, 1}, channelIDsForSortTest(got))
}

func TestChannelSortOptionsApplyLegacyIDSortFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	require.NoError(t, db.Create(&[]Channel{
		{Id: 1, Name: "first"},
		{Id: 2, Name: "second"},
	}).Error)

	options := NewChannelSortOptions("", "", true)

	var got []Channel
	require.NoError(t, options.Apply(db).Find(&got).Error)

	require.Equal(t, []int{2, 1}, channelIDsForSortTest(got))
}

func channelIDsForSortTest(channels []Channel) []int {
	ids := make([]int, 0, len(channels))
	for _, channel := range channels {
		ids = append(ids, channel.Id)
	}
	return ids
}

func int64PtrForChannelSortTest(value int64) *int64 {
	return &value
}
