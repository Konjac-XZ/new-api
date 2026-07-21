import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getOutputSpeed, getTtftMs } from './lib'
import type { MonitorRecord } from './types'

describe('monitor timing helpers', () => {
  test('measures TTFT from request start across retries', () => {
    const requestStartedAt = 1_711_456_789_000
    const currentAttemptStartedAt = requestStartedAt + 5_000
    const streamingStartedAt = currentAttemptStartedAt + 1_500

    const record: MonitorRecord = {
      id: 'req-retry-ttft',
      is_stream: true,
      start_time_ms: requestStartedAt,
      current_attempt_started_at_ms: currentAttemptStartedAt,
      current_attempt_streaming_started_at_ms: streamingStartedAt,
      retry_count: 1,
    }

    assert.equal(getTtftMs(record), 6_500)
  })

  test('does not show TTFT for non-stream requests', () => {
    const record: MonitorRecord = {
      id: 'req-non-stream',
      is_stream: false,
      start_time_ms: 1_711_456_789_000,
      current_attempt_streaming_started_at_ms: 1_711_456_790_500,
    }

    assert.equal(getTtftMs(record), null)
  })

  test('hides unreasonable streaming throughput above 5000 tokens per second', () => {
    const startedAt = 1_711_456_789_000
    const record: MonitorRecord = {
      id: 'req-fast-stream',
      is_stream: true,
      completion_tokens: 501,
      current_attempt_streaming_started_at_ms: startedAt,
      end_time_ms: startedAt + 100,
    }

    assert.equal(getOutputSpeed(record, startedAt + 100), null)
  })

  test('keeps streaming throughput at the 5000 tokens per second threshold', () => {
    const startedAt = 1_711_456_789_000
    const record: MonitorRecord = {
      id: 'req-threshold-stream',
      is_stream: true,
      completion_tokens: 500,
      current_attempt_streaming_started_at_ms: startedAt,
      end_time_ms: startedAt + 100,
    }

    assert.equal(getOutputSpeed(record, startedAt + 100), 5000)
  })
})
