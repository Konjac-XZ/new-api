# Probe Testing (探针测试) Reference

This document describes the design, database schema, configuration, and backend execution flow for Probe Testing (定时测试/探针测试) in the AI API Gateway.

## Database Schema: `channel_test_configs`

The test-specific settings for each channel are stored in the `channel_test_configs` table.

| Column | Type | Description |
| :--- | :--- | :--- |
| `channel_id` | `bigint` (PK) | Foreign key referencing `channels.id`. |
| `test_case` | `text` | Custom request prompt to send to the channel during test runs. |
| `expected_answer` | `text` | Expected text or regex pattern to validate the model response. |
| `max_first_token_latency` | `bigint` | Maximum time allowed (seconds) before the first token is received. **Must be > 0 for scheduled tests to run.** |
| `scheduled_test_interval` | `bigint` | Interval in **minutes** between consecutive scheduled probe tests. Set to `0` to disable scheduled tests. |
| `max_retry_attempts` | `bigint` | Number of retry attempts if a channel request fails during normal relay routing. |
| `treat_empty_reply_as_failure` | `boolean` | If set to `true`, empty responses from the channel are treated as test/relay failures. |

---

## Execution Logic & Guard Conditions

Scheduled tests are controlled by a background loop running in `controller/channel-test.go` (`ScheduledTestChannels`).

### 1. The Scheduling Loop
- Runs continuously on a **1-minute interval** (`time.Sleep(1 * time.Minute)`).
- Queries candidate channels using `model.GetChannelsWithScheduledTest()`, which matches channels satisfying:
  - `channels.status != 2` (i.e. not manually disabled).
  - `channel_test_configs.scheduled_test_interval > 0`.

### 2. Guard Conditions for Test Dispatch
For each candidate channel, the following conditions are verified:
- **System Activity Guard**: If the gateway has not received any live client LLM requests in the last 1 hour, all scheduled tests are globally skipped to conserve upstream quota (logged as `scheduled tests skipped: system idle (>1 hour)`).
- **Auto Ban Guard**: The channel's individual auto-ban option must be active (`channel.GetAutoBan()` is true, stored in `channels.auto_ban`).
- **Global Flag Guard**: `common.AutomaticDisableChannelEnabled` must be enabled.
- **Latency Threshold Guard**: The channel **must** have a configured `max_first_token_latency > 0`. If it is `0` or unset, the scheduled test is silently skipped (logged as `max_first_token_latency_not_configured`).
- **Circuit Breaker Status Guard**: If the channel is in a cooldown or observation period under the dynamic circuit breaker, scheduled tests may be skipped to avoid interfering with breaker recovery logic.

---

## Key SQL Operations

### 1. View Test Configurations for all Active Breaker Channels
```sql
select 
  c.id, c.name, c.status,
  t.scheduled_test_interval, t.max_first_token_latency, t.treat_empty_reply_as_failure
from channels c
join channel_breaker_states b on c.id = b.channel_id
left join channel_test_configs t on c.id = t.channel_id
where b.dynamic_circuit_breaker = true;
```

### 2. Configure Probe Test Interval (e.g. Set to 15 mins for Gemini channels)
```sql
begin;
update channel_test_configs t
set scheduled_test_interval = 15
from channels c
where t.channel_id = c.id
  and c.name ilike '%gemini%';
commit;
```

### 3. Toggle Empty Reply as Failure for all Dynamic Breaker Channels
```sql
begin;
update channel_test_configs t
set treat_empty_reply_as_failure = true
from channel_breaker_states b
where t.channel_id = b.channel_id
  and b.dynamic_circuit_breaker = true;
commit;
```
