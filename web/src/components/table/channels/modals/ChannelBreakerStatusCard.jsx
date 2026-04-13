import React, { useEffect, useMemo, useState } from 'react';
import {
  Card,
  Col,
  Collapse,
  Divider,
  Empty,
  Progress,
  Row,
  Space,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';

const { Text } = Typography;

const failureLabelMap = {
  generic: '通用失败',
  immediate_failure: '即时失败',
  first_token_timeout: '首 Token 超时',
  mid_stream_failure: '流中断失败',
  overloaded: '上游过载',
  empty_reply: '空回复',
};

const eventLabelMap = {
  relay_failure: '请求失败处罚',
  probe_failure: '探测失败处罚',
};

const traceFieldLabelMap = {
  failure_kind: '失败类型',
  pressure_before: '处罚前压力值',
  pressure_after: '处罚后压力值',
  fail_streak_before: '处罚前连续失败',
  fail_streak_after: '处罚后连续失败',
  trip_count_before: '处罚前熔断次数',
  trip_count_after: '处罚后熔断次数',
  hp_before: '处罚前 HP',
  hp_damage: 'HP 扣减',
  hp_after: '处罚后 HP',
  was_in_probation: '观察期内',
  was_awaiting_probe: '待探测阶段',
  force_cooldown: '强制冷却',
  cooldown_at_before: '处罚前冷却到期',
  cooldown_at_after: '处罚后冷却到期',
  base_cooldown_seconds: '基础冷却时长',
  cooldown_multiplier: '冷却倍数',
  chronic_floor_seconds: '慢性惩罚下限',
  final_cooldown_seconds: '最终冷却时长',
  triggered_cooldown: '是否触发冷却',
  short_term_penalty_factor: '短期惩罚因子',
  pressure_penalty_factor: '压力惩罚因子',
  history_penalty_factor: '历史惩罚因子',
  failure_rate: '近期失败率',
  timeout_rate: '近期超时率',
  confidence: '统计置信度',
  chronic_trip_floor_seconds: '熔断次数地板',
  chronic_failure_rate_floor_seconds: '失败率地板',
  chronic_streak_floor_seconds: '连续失败地板',
};

const traceFieldOrder = [
  'failure_kind',
  'pressure_before',
  'pressure_after',
  'fail_streak_before',
  'fail_streak_after',
  'trip_count_before',
  'trip_count_after',
  'hp_before',
  'hp_damage',
  'hp_after',
  'was_in_probation',
  'was_awaiting_probe',
  'force_cooldown',
  'cooldown_at_before',
  'cooldown_at_after',
  'base_cooldown_seconds',
  'cooldown_multiplier',
  'chronic_floor_seconds',
  'final_cooldown_seconds',
  'triggered_cooldown',
  'short_term_penalty_factor',
  'pressure_penalty_factor',
  'history_penalty_factor',
  'failure_rate',
  'timeout_rate',
  'confidence',
  'chronic_trip_floor_seconds',
  'chronic_failure_rate_floor_seconds',
  'chronic_streak_floor_seconds',
];

const formatUnixTime = (unixSeconds) => {
  if (!unixSeconds || unixSeconds <= 0) {
    return '-';
  }
  return new Date(unixSeconds * 1000).toLocaleString();
};

const formatSeconds = (seconds) => {
  if (!seconds || seconds <= 0) {
    return '0s';
  }
  const day = Math.floor(seconds / 86400);
  const hour = Math.floor((seconds % 86400) / 3600);
  const minute = Math.floor((seconds % 3600) / 60);
  const second = seconds % 60;
  const parts = [];
  if (day > 0) parts.push(`${day}d`);
  if (hour > 0) parts.push(`${hour}h`);
  if (minute > 0) parts.push(`${minute}m`);
  if (second > 0 || parts.length === 0) parts.push(`${second}s`);
  return parts.join(' ');
};

const formatTraceNumber = (value) => {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return String(value);
  }
  if (Number.isInteger(value)) {
    return String(value);
  }
  return value.toFixed(4).replace(/\.0+$|(?<=\.[0-9]*?)0+$/u, '').replace(/\.$/, '');
};

const normalizeUnixSeconds = (value) => {
  if (typeof value !== 'number' || Number.isNaN(value) || value <= 0) {
    return null;
  }
  // Accept both seconds and milliseconds; convert ms to seconds.
  if (value > 1e12) {
    return Math.floor(value / 1000);
  }
  return Math.floor(value);
};

const isTimestampLikeKey = (key) => {
  const normalized = String(key || '').toLowerCase().replace(/[\s.-]+/g, '_');
  if (normalized.endsWith('_at') || normalized.endsWith('_time') || normalized.endsWith('_timestamp')) {
    return true;
  }
  if (normalized.includes('expire') || normalized.includes('expired') || normalized.includes('expiry')) {
    return true;
  }
  if (normalized.includes('deadline') || normalized.includes('until')) {
    return true;
  }
  if (normalized.includes('cooldown') && (normalized.includes('at') || normalized.includes('time'))) {
    return true;
  }
  return false;
};

const looksLikeUnixTimestamp = (value) => {
  const seconds = normalizeUnixSeconds(value);
  if (seconds == null) {
    return false;
  }
  // Roughly between 2001 and year 2286 in seconds.
  return seconds >= 1e9 && seconds <= 9999999999;
};

const toReadableFieldLabel = (key) => {
  const cleaned = String(key || '')
    .replace(/^[._\s-]+/, '')
    .replace(/[._-]+/g, ' ')
    .trim();
  if (cleaned === '') {
    return String(key || '');
  }
  return cleaned
    .split(/\s+/)
    .map((segment) => segment.charAt(0).toUpperCase() + segment.slice(1))
    .join(' ');
};

const formatTraceValue = (key, value, t) => {
  if (value == null) {
    return '-';
  }
  if (typeof value === 'boolean') {
    return value ? t('是') : t('否');
  }
  if (typeof value === 'number') {
    if (key.endsWith('_seconds')) {
      return formatSeconds(value);
    }
    if (isTimestampLikeKey(key) && looksLikeUnixTimestamp(value)) {
      return formatUnixTime(normalizeUnixSeconds(value));
    }
    if (key.includes('rate')) {
      return `${(value * 100).toFixed(1)}%`;
    }
    return formatTraceNumber(value);
  }
  if (typeof value === 'string') {
    const parsed = Number(value);
    if (!Number.isNaN(parsed) && isTimestampLikeKey(key) && looksLikeUnixTimestamp(parsed)) {
      return formatUnixTime(normalizeUnixSeconds(parsed));
    }
  }
  return String(value);
};

const getTraceFieldLabel = (key, t) => {
  const mapped = traceFieldLabelMap[key];
  if (mapped) {
    return t(mapped);
  }
  return toReadableFieldLabel(key);
};

const sortTraceEntries = (payload) => {
  return Object.entries(payload || {}).sort(([left], [right]) => {
    const leftIndex = traceFieldOrder.indexOf(left);
    const rightIndex = traceFieldOrder.indexOf(right);
    if (leftIndex === -1 && rightIndex === -1) {
      return left.localeCompare(right);
    }
    if (leftIndex === -1) {
      return 1;
    }
    if (rightIndex === -1) {
      return -1;
    }
    return leftIndex - rightIndex;
  });
};

const getPhaseMeta = (phase, t) => {
  switch (phase) {
    case 'cooling':
      return {
        color: 'red',
        label: t('冷却中'),
        description: t('该渠道已临时移出候选列表，等待冷却结束'),
      };
    case 'observation':
      return {
        color: 'orange',
        label: t('观察期'),
        description: t('冷却已结束，下一次成功请求将恢复到正常状态'),
      };
    case 'awaiting_probe':
      return {
        color: 'yellow',
        label: t('待探测'),
        description: t('冷却已结束，等待最近一次探测成功后进入观察期'),
      };
    case 'closed':
      return {
        color: 'green',
        label: t('正常'),
        description: t('动态熔断已启用，当前未处于冷却'),
      };
    default:
      return {
        color: 'grey',
        label: t('未启用'),
        description: t('需同时开启自动禁用与动态熔断冷却'),
      };
  }
};

const TraceFieldGrid = ({ payload, t }) => {
  const entries = sortTraceEntries(payload);
  if (entries.length === 0) {
    return <Text type='tertiary'>{t('暂无数据')}</Text>;
  }

  return (
    <div className='grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-2'>
      {entries.map(([key, value]) => (
        <div key={key}>
          <Text size='small' type='tertiary'>
            {getTraceFieldLabel(key, t)}
          </Text>
          <div>
            <Text>{formatTraceValue(key, value, t)}</Text>
          </div>
        </div>
      ))}
    </div>
  );
};

const TraceSummaryGrid = ({ trace, t }) => {
  const summaryItems = [
    ['处罚前 HP', formatTraceNumber(Number(trace?.hp_before || 0))],
    ['HP 扣减', formatTraceNumber(Number(trace?.hp_damage || 0))],
    ['处罚后 HP', formatTraceNumber(Number(trace?.hp_after || 0))],
    ['处罚前压力值', formatTraceNumber(Number(trace?.pressure_before || 0))],
    ['处罚后压力值', formatTraceNumber(Number(trace?.pressure_after || 0))],
    ['处罚前连续失败', trace?.fail_streak_before || 0],
    ['处罚后连续失败', trace?.fail_streak_after || 0],
    ['处罚前熔断次数', trace?.trip_count_before || 0],
    ['处罚后熔断次数', trace?.trip_count_after || 0],
    ['基础冷却时长', formatSeconds(Number(trace?.base_cooldown_seconds || 0))],
    ['冷却倍数', formatTraceNumber(Number(trace?.cooldown_multiplier || 0))],
    ['最终冷却时长', formatSeconds(Number(trace?.final_cooldown_seconds || 0))],
  ];

  return (
    <div className='grid grid-cols-1 md:grid-cols-2 gap-x-4 gap-y-2'>
      {summaryItems.map(([label, value]) => (
        <div key={label}>
          <Text size='small' type='tertiary'>
            {t(label)}
          </Text>
          <div>
            <Text>{value}</Text>
          </div>
        </div>
      ))}
    </div>
  );
};

const ChannelBreakerStatusCard = ({
  breakerState,
  t,
  visible,
  traces = [],
  traceTotal = 0,
  traceLoading = false,
}) => {
  const [nowSeconds, setNowSeconds] = useState(Math.floor(Date.now() / 1000));

  useEffect(() => {
    if (!visible) {
      return undefined;
    }
    const timer = setInterval(() => {
      setNowSeconds(Math.floor(Date.now() / 1000));
    }, 1000);
    return () => clearInterval(timer);
  }, [visible]);

  const phase = breakerState?.phase || 'disabled';
  const phaseMeta = useMemo(() => getPhaseMeta(phase, t), [phase, t]);
  const remainingCooldownSeconds = Math.max(
    0,
    (breakerState?.cooldown_at || 0) - nowSeconds,
  );
  const totalCooldownSeconds =
    breakerState?.cooldown_seconds && breakerState.cooldown_seconds > 0
      ? breakerState.cooldown_seconds
      : 0;
  const cooldownPercent =
    totalCooldownSeconds > 0
      ? Math.max(
        0,
        Math.min(
          100,
          (remainingCooldownSeconds / totalCooldownSeconds) * 100,
        ),
      )
      : 0;
  const pressureRaw = Number(breakerState?.pressure || 0);
  const pressurePercent = Math.max(0, Math.min(100, (pressureRaw / 8) * 100));
  const failureKind = breakerState?.last_failure || '';
  const failureLabel = failureKind
    ? failureLabelMap[failureKind] || failureKind
    : t('无');

  // HP data
  const hpMax = Number(breakerState?.max_hp || 10);
  const hpValue = breakerState?.hp != null ? Number(breakerState.hp) : hpMax;
  const hpPercent = hpMax > 0 ? Math.max(0, Math.min(100, (hpValue / hpMax) * 100)) : 100;
  const hpStroke = hpPercent > 60 ? '#22c55e' : hpPercent > 30 ? '#f59e0b' : '#ef4444';
  const isDynamicEnabled = phase !== 'disabled';

  return (
    <Card
      bordered
      style={{
        marginBottom: 16,
        borderRadius: 12,
        background:
          'linear-gradient(135deg, var(--semi-color-bg-0) 0%, var(--semi-color-bg-1) 100%)',
      }}
    >
      <div className='flex items-start justify-between gap-3'>
        <div>
          <Text strong>{t('动态熔断状态')}</Text>
          <div>
            <Text type='tertiary' size='small'>
              {phaseMeta.description}
            </Text>
          </div>
        </div>
        <Tag color={phaseMeta.color} shape='circle'>
          {phaseMeta.label}
        </Tag>
      </div>

      {phase === 'cooling' && (
        <div style={{ marginTop: 12, marginBottom: 8 }}>
          <div className='flex items-center justify-between mb-1'>
            <Text size='small'>{t('冷却进度')}</Text>
            <Text size='small' type='tertiary'>
              {t('剩余')}: {formatSeconds(remainingCooldownSeconds)}
            </Text>
          </div>
          <Progress
            percent={cooldownPercent}
            showInfo={false}
            size='small'
            stroke='#ef4444'
            style={{ height: 6, borderRadius: 999 }}
          />
        </div>
      )}

      {isDynamicEnabled && (
        <div style={{ marginTop: 12, marginBottom: 8 }}>
          <div className='flex items-center justify-between mb-1'>
            <Text size='small'>{t('当前 HP')}</Text>
            <Text size='small' type='tertiary'>
              {hpValue.toFixed(1)} / {hpMax.toFixed(1)}
            </Text>
          </div>
          <Progress
            percent={hpPercent}
            showInfo={false}
            size='small'
            stroke={hpStroke}
            style={{ height: 6, borderRadius: 999 }}
          />
        </div>
      )}

      <Row gutter={12} style={{ marginTop: 8 }}>
        <Col span={12}>
          <Text size='small' type='tertiary'>
            {t('压力值')}
          </Text>
          <div style={{ marginTop: 4 }}>
            <Text strong>{pressureRaw.toFixed(2)}</Text>
          </div>
        </Col>
        <Col span={12}>
          <Text size='small' type='tertiary'>
            {t('连续失败')}
          </Text>
          <div style={{ marginTop: 4 }}>
            <Text strong>{breakerState?.fail_streak || 0}</Text>
          </div>
          <Text size='small' type='tertiary'>
            {t('最近失败类型')}: {failureLabel}
          </Text>
        </Col>
      </Row>

      <Row gutter={12} style={{ marginTop: 10 }}>
        <Col span={12}>
          <Text size='small' type='tertiary'>
            {t('冷却结束时间')}
          </Text>
          <div>
            <Text>{formatUnixTime(breakerState?.cooldown_at || 0)}</Text>
          </div>
        </Col>
        <Col span={12}>
          <Text size='small' type='tertiary'>
            {t('最近状态更新时间')}
          </Text>
          <div>
            <Text>{formatUnixTime(breakerState?.updated_at || 0)}</Text>
          </div>
        </Col>
      </Row>

      {isDynamicEnabled && (
        <Row gutter={12} style={{ marginTop: 10 }}>
          <Col span={12}>
            <Text size='small' type='tertiary'>
              {t('熔断次数')}
            </Text>
            <div style={{ marginTop: 4 }}>
              <Text strong>{breakerState?.trip_count || 0}</Text>
            </div>
          </Col>
          <Col span={12}>
            <Text size='small' type='tertiary'>
              {t('容错系数')}
            </Text>
            <div style={{ marginTop: 4 }}>
              <Text strong>{(breakerState?.tolerance_coefficient || 1.0).toFixed(1)}</Text>
            </div>
          </Col>
        </Row>
      )}

      {isDynamicEnabled && (
        <Row gutter={12} style={{ marginTop: 10 }}>
          <Col span={12}>
            <Text size='small' type='tertiary'>
              {t('近期失败率')}
            </Text>
            <div style={{ marginTop: 4 }}>
              <Text strong>{((breakerState?.failure_rate || 0) * 100).toFixed(1)}%</Text>
            </div>
          </Col>
          <Col span={12}>
            <Text size='small' type='tertiary'>
              {t('近期超时率')}
            </Text>
            <div style={{ marginTop: 4 }}>
              <Text strong>{((breakerState?.timeout_rate || 0) * 100).toFixed(1)}%</Text>
            </div>
          </Col>
        </Row>
      )}

      {phase === 'observation' && (
        <div style={{ marginTop: 10 }}>
          <Text size='small' type='tertiary'>
            {t('观察期已持续')}: {formatSeconds(
              breakerState?.observation_elapsed_seconds || 0,
            )}
          </Text>
        </div>
      )}

      <Divider style={{ margin: '16px 0 12px' }} />

      <div>
        <div className='flex items-center justify-between gap-3'>
          <Text strong>{t('最近处罚计算记录')}</Text>
          <Text size='small' type='tertiary'>
            {t('最近 {{count}} / 共 {{total}} 条', {
              count: traces.length,
              total: traceTotal,
            })}
          </Text>
        </div>

        {traceLoading && traces.length === 0 ? (
          <div style={{ marginTop: 12 }}>
            <Text type='tertiary'>{t('加载中...')}</Text>
          </div>
        ) : traces.length === 0 ? (
          <div style={{ marginTop: 12 }}>
            <Empty description={t('暂无处罚计算记录')} />
          </div>
        ) : (
          <div style={{ marginTop: 12 }}>
            <Collapse accordion>
              {traces.map((trace) => {
                const eventLabel = t(eventLabelMap[trace?.event_type] || trace?.event_type || '处罚记录');
                const failureLabel = trace?.failure_kind
                  ? t(failureLabelMap[trace.failure_kind] || trace.failure_kind)
                  : t('无');
                const finalCooldown = formatSeconds(Number(trace?.final_cooldown_seconds || 0));

                return (
                  <Collapse.Panel
                    key={trace.id}
                    itemKey={String(trace.id)}
                    header={
                      <Space wrap spacing={8}>
                        <Tag color='blue'>{eventLabel}</Tag>
                        <Tag color={trace?.triggered_cooldown ? 'red' : 'grey'}>
                          {trace?.triggered_cooldown
                            ? t('已触发冷却')
                            : t('未触发冷却')}
                        </Tag>
                        <Text strong>{failureLabel}</Text>
                        <Text type='tertiary' size='small'>
                          {formatUnixTime(trace?.created_at || 0)}
                        </Text>
                        <Text type='tertiary' size='small'>
                          {t('最终冷却时长')}: {finalCooldown}
                        </Text>
                      </Space>
                    }
                  >
                    <div className='flex flex-col gap-4'>
                      <TraceSummaryGrid trace={trace} t={t} />

                      <div>
                        <Text strong>{t('计算参数')}</Text>
                        <div style={{ marginTop: 8 }}>
                          <TraceFieldGrid payload={trace?.calculation_inputs} t={t} />
                        </div>
                      </div>

                      <div>
                        <Text strong>{t('计算步骤')}</Text>
                        <div className='mt-2 flex flex-col gap-2'>
                          {(trace?.calculation_steps || []).length > 0 ? (
                            trace.calculation_steps.map((step, index) => (
                              <div
                                key={`${trace.id}-step-${index}`}
                                className='rounded-lg px-3 py-2 text-xs leading-5 whitespace-pre-wrap break-all'
                                style={{
                                  background: 'var(--semi-color-fill-0)',
                                  fontFamily: 'var(--semi-font-family-monospace)',
                                }}
                              >
                                {step}
                              </div>
                            ))
                          ) : (
                            <Text type='tertiary'>{t('暂无数据')}</Text>
                          )}
                        </div>
                      </div>

                      <div>
                        <Text strong>{t('计算结果')}</Text>
                        <div style={{ marginTop: 8 }}>
                          <TraceFieldGrid payload={trace?.calculation_result} t={t} />
                        </div>
                      </div>
                    </div>
                  </Collapse.Panel>
                );
              })}
            </Collapse>
          </div>
        )}
      </div>
    </Card>
  );
};

export default ChannelBreakerStatusCard;
