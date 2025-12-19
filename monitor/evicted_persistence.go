package monitor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type EvictedPersistenceConfig struct {
	Enabled     bool
	Dir         string
	FlushDelay  time.Duration
	ChannelSize int
	MaxBatch    int
	PurgeAt     string // "HH:MM" local time
}

func loadEvictedPersistenceConfigFromEnv() EvictedPersistenceConfig {
	enabled := common.GetEnvOrDefaultBool("MONITOR_EVICT_PERSIST_ENABLED", false)

	dir := common.GetEnvOrDefaultString("MONITOR_EVICT_PERSIST_DIR", "./data/monitor-evicted")
	delay := parseDurationEnv("MONITOR_EVICT_PERSIST_FLUSH_DELAY", 30*time.Second)
	channelSize := common.GetEnvOrDefault("MONITOR_EVICT_PERSIST_CHANNEL_SIZE", 4096)
	maxBatch := common.GetEnvOrDefault("MONITOR_EVICT_PERSIST_MAX_BATCH", 256)
	purgeAt := common.GetEnvOrDefaultString("MONITOR_EVICT_PERSIST_PURGE_AT", "04:00")

	if channelSize < 1 {
		channelSize = 1
	}
	if maxBatch < 1 {
		maxBatch = 1
	}
	if delay < 0 {
		delay = 0
	}

	return EvictedPersistenceConfig{
		Enabled:     enabled,
		Dir:         dir,
		FlushDelay:  delay,
		ChannelSize: channelSize,
		MaxBatch:    maxBatch,
		PurgeAt:     purgeAt,
	}
}

func parseDurationEnv(env string, defaultValue time.Duration) time.Duration {
	raw := common.GetEnvOrDefaultString(env, "")
	if strings.TrimSpace(raw) == "" {
		return defaultValue
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to parse %s=%q as duration: %v, using default %s", env, raw, err, defaultValue))
		return defaultValue
	}
	return parsed
}

type evictedItem struct {
	EnqueuedAt time.Time
	Record     *RequestRecord
}

type hourBucket struct {
	date string // yyyy-mm-dd
	hour string // HH
}

type hourWriter struct {
	dir      string
	filename string
	file     *os.File
	writer   *bufio.Writer
	lastUsed time.Time
}

// EvictedRecordPersister persists records that are evicted from the in-memory FIFO/ring buffer.
//
// Design goals:
// - Never block request processing paths (non-blocking enqueue).
// - Do disk IO in a background goroutine.
// - Batch writes with a configurable delay.
// - Purge all on-disk data once per day at a configurable time.
type EvictedRecordPersister struct {
	cfg EvictedPersistenceConfig

	in     chan evictedItem
	purge  chan struct{}
	once   sync.Once
	fileMu sync.Mutex

	writers map[hourBucket]*hourWriter

	purgeHour   int
	purgeMinute int

	droppedMu sync.Mutex
	dropped   int64
}

func NewEvictedRecordPersister(cfg EvictedPersistenceConfig) (*EvictedRecordPersister, error) {
	h, m, err := parseHHMM(cfg.PurgeAt)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, fmt.Errorf("MONITOR_EVICT_PERSIST_DIR must not be empty")
	}
	return &EvictedRecordPersister{
		cfg:         cfg,
		in:          make(chan evictedItem, cfg.ChannelSize),
		purge:       make(chan struct{}, 1),
		writers:     make(map[hourBucket]*hourWriter, 32),
		purgeHour:   h,
		purgeMinute: m,
	}, nil
}

func (p *EvictedRecordPersister) Start() {
	p.once.Do(func() {
		if err := os.MkdirAll(p.cfg.Dir, 0o755); err != nil {
			common.SysError("failed to create monitor evicted persistence dir: " + err.Error())
		}
		go p.run()
		go p.runPurgeScheduler()
	})
}

// OnEvicted is called from the hot path (request recording). It must be non-blocking.
func (p *EvictedRecordPersister) OnEvicted(record *RequestRecord) {
	if p == nil || record == nil {
		return
	}
	item := evictedItem{
		EnqueuedAt: time.Now(),
		Record:     record,
	}

	select {
	case p.in <- item:
	default:
		p.droppedMu.Lock()
		p.dropped++
		p.droppedMu.Unlock()
	}
}

func (p *EvictedRecordPersister) run() {
	var buffer []evictedItem
	var timer *time.Timer
	var lastPurge time.Time

	flush := func(items []evictedItem) {
		if len(items) == 0 {
			return
		}
		// Filter out anything enqueued before/at the last purge moment.
		filtered := items[:0]
		for _, it := range items {
			if !lastPurge.IsZero() && !it.EnqueuedAt.After(lastPurge) {
				continue
			}
			if it.Record == nil {
				continue
			}
			filtered = append(filtered, it)
		}
		if len(filtered) == 0 {
			return
		}

		p.fileMu.Lock()
		defer p.fileMu.Unlock()

		if err := p.flushToDiskLocked(filtered); err != nil {
			common.SysError("failed to persist evicted monitor records: " + err.Error())
		}
	}

	resetTimer := func() {
		if p.cfg.FlushDelay <= 0 {
			return
		}
		if timer == nil {
			timer = time.NewTimer(p.cfg.FlushDelay)
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(p.cfg.FlushDelay)
	}

	for {
		var timerCh <-chan time.Time
		if timer != nil {
			timerCh = timer.C
		}

		select {
		case item := <-p.in:
			if !lastPurge.IsZero() && !item.EnqueuedAt.After(lastPurge) {
				continue
			}
			buffer = append(buffer, item)
			if len(buffer) >= p.cfg.MaxBatch || p.cfg.FlushDelay <= 0 {
				flush(buffer)
				buffer = buffer[:0]
				if timer != nil {
					timer.Stop()
					timer = nil
				}
				continue
			}
			resetTimer()

		case <-timerCh:
			flush(buffer)
			buffer = buffer[:0]
			if timer != nil {
				timer.Stop()
				timer = nil
			}

		case <-p.purge:
			lastPurge = time.Now()
			buffer = buffer[:0]
			if timer != nil {
				timer.Stop()
				timer = nil
			}

			p.fileMu.Lock()
			_ = p.closeAllWritersLocked()
			if err := purgeDirContents(p.cfg.Dir); err != nil {
				common.SysError("failed to purge monitor evicted persistence dir: " + err.Error())
			}
			p.fileMu.Unlock()
		}
	}
}

func (p *EvictedRecordPersister) flushToDiskLocked(items []evictedItem) error {
	if err := os.MkdirAll(p.cfg.Dir, 0o755); err != nil {
		return err
	}

	// Group by local day/hour so we get an on-disk layout:
	//   <dir>/yyyy-mm-dd/HH/evicted.jsonl
	// This provides good separation while still using append-only writes for throughput.
	buckets := make(map[hourBucket][]*RequestRecord, 8)
	for _, it := range items {
		if it.Record == nil {
			continue
		}
		t := it.EnqueuedAt.Local()
		key := hourBucket{
			date: t.Format("2006-01-02"),
			hour: t.Format("15"),
		}
		buckets[key] = append(buckets[key], it.Record)
	}

	for key, records := range buckets {
		if len(records) == 0 {
			continue
		}

		hw, err := p.getWriterLocked(key)
		if err != nil {
			return err
		}
		for _, r := range records {
			b, err := json.Marshal(r)
			if err != nil {
				continue
			}
			if _, err := hw.writer.Write(b); err != nil {
				return err
			}
			if err := hw.writer.WriteByte('\n'); err != nil {
				return err
			}
		}
		// Flush per bucket on each batch flush so data hits disk promptly,
		// while still avoiding repeated open/close syscalls.
		if err := hw.writer.Flush(); err != nil {
			return err
		}
	}

	return nil
}

func (p *EvictedRecordPersister) getWriterLocked(key hourBucket) (*hourWriter, error) {
	now := time.Now()
	if existing := p.writers[key]; existing != nil && existing.file != nil && existing.writer != nil {
		existing.lastUsed = now
		return existing, nil
	}

	dir := filepath.Join(p.cfg.Dir, key.date, key.hour)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	filename := filepath.Join(dir, "evicted.jsonl")

	// Append-only hourly file: highest throughput for this access pattern.
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	// Larger buffer reduces syscalls further when batching records.
	w := bufio.NewWriterSize(f, 4<<20) // 4MB
	hw := &hourWriter{
		dir:      dir,
		filename: filename,
		file:     f,
		writer:   w,
		lastUsed: now,
	}
	p.writers[key] = hw
	return hw, nil
}

func (p *EvictedRecordPersister) closeAllWritersLocked() error {
	var firstErr error
	for key, hw := range p.writers {
		if hw == nil {
			delete(p.writers, key)
			continue
		}
		if hw.writer != nil {
			if err := hw.writer.Flush(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if hw.file != nil {
			if err := hw.file.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(p.writers, key)
	}
	return firstErr
}

func (p *EvictedRecordPersister) runPurgeScheduler() {
	for {
		next := nextLocalTime(time.Now(), p.purgeHour, p.purgeMinute)
		time.Sleep(time.Until(next))
		// Let the run loop do the purge so it can also clear in-memory buffers.
		p.purge <- struct{}{}
	}
}

func parseHHMM(value string) (int, int, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0, 0, fmt.Errorf("purge time must not be empty (expected HH:MM)")
	}
	parts := strings.Split(v, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid purge time %q (expected HH:MM)", value)
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("invalid purge hour in %q", value)
	}
	min, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || min < 0 || min > 59 {
		return 0, 0, fmt.Errorf("invalid purge minute in %q", value)
	}
	return hour, min, nil
}

func nextLocalTime(now time.Time, hour, minute int) time.Time {
	local := now.Local()
	target := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, local.Location())
	if !target.After(local) {
		target = target.Add(24 * time.Hour)
	}
	return target
}

func purgeDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
