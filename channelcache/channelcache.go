package channelcache

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type cacheEntry struct {
	name     string
	loadedAt time.Time
}

var (
	nameTTL        = 30 * time.Minute
	channelNameMap sync.Map // map[int]cacheEntry
)

// Remember stores id->name mappings that we already have, refreshing the cache TTL.
func Remember(channelID int, name string) {
	if channelID <= 0 {
		return
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	channelNameMap.Store(channelID, cacheEntry{name: name, loadedAt: time.Now()})
}

// Name returns the cached name for the given channel ID, fetching from DB on cache miss.
func Name(channelID int) string {
	return nameOrFallback(channelID, "")
}

// NameOr returns the cached name or the provided fallback if we cannot resolve one.
func NameOr(channelID int, fallback string) string {
	name := nameOrFallback(channelID, fallback)
	if name == "" {
		return fallback
	}
	return name
}

// Label formats a consistent `channel #ID (name)` label, falling back to the provided hint.
func Label(channelID int, fallback string) string {
	name := NameOr(channelID, fallback)
	switch {
	case channelID > 0 && name != "":
		return fmt.Sprintf("channel #%d (%s)", channelID, name)
	case channelID > 0:
		return fmt.Sprintf("channel #%d", channelID)
	case name != "":
		return fmt.Sprintf("channel (%s)", name)
	default:
		return "channel <unknown>"
	}
}

func nameOrFallback(channelID int, fallback string) string {
	if channelID <= 0 {
		return strings.TrimSpace(fallback)
	}
	if val, ok := channelNameMap.Load(channelID); ok {
		entry := val.(cacheEntry)
		if entry.name != "" && time.Since(entry.loadedAt) < nameTTL {
			return entry.name
		}
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil || channel == nil {
		return strings.TrimSpace(fallback)
	}
	name := strings.TrimSpace(channel.Name)
	if name != "" {
		channelNameMap.Store(channelID, cacheEntry{name: name, loadedAt: time.Now()})
		return name
	}
	return strings.TrimSpace(fallback)
}
