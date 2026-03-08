import React, { useEffect, useMemo, useState } from 'react';
import { Card, Col, Progress, Row, Tag, Typography } from '@douyinfe/semi-ui';

const { Text } = Typography;

const failureLabelMap = {
  generic: '通用失败',
  immediate_failure: '即时失败',
  first_token_timeout: '首 Token 超时',
  mid_stream_failure: '流中断失败',
  overloaded: '上游过载',
  empty_reply: '空回复',
};

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

const ChannelBreakerStatusCard = ({ breakerState, t, visible }) => {
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
            ((totalCooldownSeconds - remainingCooldownSeconds) /
              totalCooldownSeconds) *
              100,
          ),
        )
      : 0;
  const pressureRaw = Number(breakerState?.pressure || 0);
  const pressurePercent = Math.max(0, Math.min(100, (pressureRaw / 8) * 100));
  const failureKind = breakerState?.last_failure || '';
  const failureLabel = failureKind
    ? failureLabelMap[failureKind] || failureKind
    : t('无');

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

      <Row gutter={12} style={{ marginTop: 8 }}>
        <Col span={12}>
          <Text size='small' type='tertiary'>
            {t('压力值')}
          </Text>
          <div className='flex items-center justify-between mb-1'>
            <Text strong>{pressureRaw.toFixed(2)}</Text>
            <Text size='small' type='tertiary'>
              / 8.00
            </Text>
          </div>
          <Progress
            percent={pressurePercent}
            showInfo={false}
            size='small'
            stroke={phase === 'closed' ? '#22c55e' : '#f59e0b'}
            style={{ height: 6, borderRadius: 999 }}
          />
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

      {phase === 'observation' && (
        <div style={{ marginTop: 10 }}>
          <Text size='small' type='tertiary'>
            {t('观察期已持续')}: {formatSeconds(
              breakerState?.observation_elapsed_seconds || 0,
            )}
          </Text>
        </div>
      )}
    </Card>
  );
};

export default ChannelBreakerStatusCard;
