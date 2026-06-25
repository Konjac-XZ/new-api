---
name: channels-table-access
description: Safely access, inspect, and maintain the channels, channel_breaker_states, and channel_test_configs tables in the local new-api database.
---

# Channels Table Access & Maintenance

## Overview

The following three tables are the most frequently modified tables in this project's database:
1. `channels`: The main channel configuration (credentials, models, status, ratios, priority).
2. `channel_breaker_states`: Stores dynamic circuit breaker/fuse states and metrics.
3. `channel_test_configs`: Stores channel test parameters, timeout limits (e.g., first token latency), and retry limits.

Use the repository `.envrc` as the source of truth for the local database connection. The live local app uses PostgreSQL through `SQL_DSN`.

## Connection

Work from the repository root:

```bash
cd /home/xinrui/GitHub/new-api
set -a; . ./.envrc; set +a
psql "$SQL_DSN"
```

One-off command example:
```bash
set -a; . ./.envrc; set +a
psql "$SQL_DSN" -c "select id, name, status from channels limit 5;"
```

## Safety Rules

1. **Select first**: Never run an update/delete without verifying the target rows with a `select` query.
2. **Mask credentials**: Never output raw values of the `key` column. Always output masked keys:
   `case when key is null or key = '' then '' else left(key, 6) || '...' || right(key, 4) end as masked_key`
3. **Use transactions**: All write operations (INSERT, UPDATE, DELETE) must run inside a transaction block (`begin; ... commit;`) and be previewed before commit.

## Querying the Three Tables

### 1. Find Channels by Name Keywords (e.g. Gemini 免费)
```sql
select id, name, type, status
from channels
where name ilike '%gemini%' and name like '%免费%'
order by id;
```

### 2. View Channel Details along with Breaker and Test Configs
```sql
select 
  c.id, c.name, c.status, c.models,
  b.dynamic_circuit_breaker, b.breaker_pressure,
  t.max_first_token_latency, t.max_retry_attempts
from channels c
left join channel_breaker_states b on c.id = b.channel_id
left join channel_test_configs t on c.id = t.channel_id
where c.id = 123;
```

### 3. Read Safe Channel List Summary
```sql
select
  id, name, type, status, models, priority, used_quota,
  case when key is null or key = '' then '' else left(key, 6) || '...' || right(key, 4) end as masked_key
from channels
order by id
limit 20;
```

## Write Operations

### 1. Update Channel Fields (with lock)
```sql
begin;
select id, name, status from channels where id = 123 for update;
-- update channels set status = 1 where id = 123;
commit;
```

### 2. Modify Breaker States (e.g., Enable/Disable dynamic circuit breaker)
```sql
begin;
update channel_breaker_states
set dynamic_circuit_breaker = true
where channel_id = 123;
commit;
```

### 3. Modify Test & Timeout Configs (e.g., Set first token timeout or retry limits)
```sql
begin;
update channel_test_configs
set max_first_token_latency = 5, max_retry_attempts = 3
where channel_id = 123;
commit;
```

### 4. Array Manipulation for comma-separated models list
```sql
begin;
-- Example: Remove specific models from Gemini 免费 channels
update channels
set models = array_to_string(array_remove(array_remove(string_to_array(models, ','), 'gemini-3-flash-preview'), 'gemini-3.5-flash'), ',')
where name ilike '%gemini%' and name like '%免费%';
commit;
```
