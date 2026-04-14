/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, {
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  useSyncExternalStore,
} from 'react';
import {
  Card,
  Table,
  Tag,
  Typography,
  Space,
  Button,
  Empty,
  Spin,
  Badge,
  Tabs,
  TabPane,
  Collapse,
  Tooltip,
  Modal,
} from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import {
  Activity,
  ArrowDownToLine,
  ArrowUpFromLine,
  Clock3,
  Copy,
  Globe2,
  Hash,
  History,
  KeyRound,
  Network,
  Radio,
  Route,
  User,
  WrapText,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import useMonitorWs from './useMonitorWs';
import useRequestDetail from './useRequestDetail';
import { useStopwatch } from './useStopwatch';
import {
  deriveDisplayStatus,
  isActiveStatus,
  isTerminalStatus,
} from './statusUtils';
import {
  renderModelTag,
  stringToColor,
  escapeHtml,
} from '../../helpers';
import { API } from '../../helpers/api';
import {
  DURATION_UPDATE_INTERVAL_MS,
  MS_TO_SECONDS,
  BODY_DISPLAY_LIMIT_BYTES,
  DURATION_INSTANT_THRESHOLD_S,
  DURATION_FAST_THRESHOLD_S,
  DURATION_MEDIUM_THRESHOLD_S,
  DURATION_SLOW_THRESHOLD_S
} from './constants';

const { Title, Text } = Typography;

const statusColors = {
  pending: 'grey',
  processing: 'blue',
  waiting_upstream: 'blue',
  streaming: 'purple',
  completed: 'green',
  error: 'red',
  abandoned: 'grey',
};

const channelPhaseColors = {
  waiting_upstream: 'blue',
  streaming: 'purple',
  error: 'red',
  completed: 'green',
};

const attemptStatusColors = {
  waiting_upstream: 'blue',
  streaming: 'purple',
  failed: 'red',
  abandoned: 'grey',
  succeeded: 'green',
};

const getTimestampMs = (msValue, fallbackValue) => {
  if (Number.isFinite(msValue) && msValue > 0) {
    return msValue;
  }

  if (!fallbackValue) {
    return 0;
  }

  const parsed = new Date(fallbackValue).getTime();
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
};

const padTo2Digits = (value) => String(value).padStart(2, '0');

const formatMonthDayTime = (msValue, fallbackValue) => {
  const timestampMs = getTimestampMs(msValue, fallbackValue);
  if (!timestampMs) {
    return '-';
  }

  const date = new Date(timestampMs);
  if (!Number.isFinite(date.getTime())) {
    return '-';
  }

  return `${padTo2Digits(date.getMonth() + 1)}-${padTo2Digits(date.getDate())} ${padTo2Digits(date.getHours())}:${padTo2Digits(date.getMinutes())}:${padTo2Digits(date.getSeconds())}`;
};

const formatLiveSeconds = (seconds) => {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return '0.0';
  }

  return (Math.floor(seconds * 10) / 10).toFixed(1);
};

const getRequestStartMs = (record) =>
  getTimestampMs(record?.start_time_ms, record?.start_time);

const getSyncedNowMs = (record, clientNowMs) => {
  const serverNowMs = record?.server_now_ms;
  const receivedAtMs = record?._receivedAtMs;

  if (
    Number.isFinite(serverNowMs) && serverNowMs > 0
    && Number.isFinite(receivedAtMs) && receivedAtMs > 0
  ) {
    return serverNowMs + Math.max(0, clientNowMs - receivedAtMs);
  }

  return clientNowMs;
};

const renderDurationTag = (durationMs, t) => {
  if (!durationMs) return <Text type='tertiary'>-</Text>;
  const seconds = Number(durationMs / MS_TO_SECONDS).toFixed(1);
  const value = parseFloat(seconds);
  let color = 'grey';

  if (value >= DURATION_SLOW_THRESHOLD_S) {
    color = 'red';
  } else if (value >= DURATION_MEDIUM_THRESHOLD_S) {
    color = 'orange';
  } else if (value >= DURATION_FAST_THRESHOLD_S) {
    color = 'blue';
  } else if (value >= DURATION_INSTANT_THRESHOLD_S) {
    color = 'green';
  }


  return (
    <Tag color={color} shape='circle'>
      {seconds}s
    </Tag>
  );
};

const formatMemory = (bytes) => {
  if (!bytes || bytes === 0) return '0B';
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
};

// Shared ticker avoids per-row intervals for active durations.
const createDurationTicker = (intervalMs) => {
  let now = Date.now();
  let timerId = null;
  const listeners = new Set();

  const tick = () => {
    now = Date.now();
    listeners.forEach((listener) => listener());
  };

  const ensureRunning = () => {
    if (timerId) return;
    timerId = setInterval(tick, intervalMs);
  };

  const stopIfIdle = () => {
    if (listeners.size > 0) return;
    if (timerId) {
      clearInterval(timerId);
      timerId = null;
    }
  };

  return {
    getSnapshot: () => now,
    subscribe: (listener) => {
      listeners.add(listener);
      if (listeners.size === 1) {
        ensureRunning();
      }
      return () => {
        listeners.delete(listener);
        stopIfIdle();
      };
    },
  };
};

const durationTicker = createDurationTicker(DURATION_UPDATE_INTERVAL_MS);

const useDurationNow = (enabled) => {
  const subscribe = useCallback(
    (listener) => {
      if (!enabled) return () => { };
      return durationTicker.subscribe(listener);
    },
    [enabled],
  );

  const getSnapshot = useCallback(
    () => (enabled ? durationTicker.getSnapshot() : 0),
    [enabled],
  );

  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
};

// Component to display duration with real-time stopwatch for ongoing requests
const DurationCell = ({ record, t }) => {
  const displayStatus = useMemo(() => deriveDisplayStatus(record), [record]);
  const isActive = useMemo(
    () => isActiveStatus(displayStatus),
    [displayStatus],
  );
  const activeStartTimeMs = useMemo(() => getRequestStartMs(record), [record]);
  const hasStartTime = activeStartTimeMs > 0;
  const now = useDurationNow(isActive && hasStartTime);

  const elapsed = useMemo(() => {
    if (!isActive || !activeStartTimeMs) return 0;
    const elapsedSeconds = (
      getSyncedNowMs(record, now) - activeStartTimeMs
    ) / MS_TO_SECONDS;
    return elapsedSeconds > 0 ? elapsedSeconds : 0;
  }, [activeStartTimeMs, isActive, now, record]);

  if (isActive) {
    return (
      <Tag color='grey' shape='circle'>
        {formatLiveSeconds(elapsed)}s
      </Tag>
    );
  }

  return renderDurationTag(record.duration_ms, t);
};

const getStatusLabels = (t) => ({
  pending: t('等待中'),
  processing: t('处理中'),
  waiting_upstream: t('等待响应'),
  streaming: t('流式返回中'),
  completed: t('完成'),
  error: t('错误'),
  abandoned: t('放弃'),
});

const getPhaseLabels = (t) => ({
  waiting_upstream: t('等待响应'),
  streaming: t('流式返回中'),
  error: t('发生错误'),
  completed: t('已完成'),
});

const getAttemptStatusLabels = (t) => ({
  waiting_upstream: t('等待响应'),
  streaming: t('流式返回中'),
  failed: t('失败'),
  abandoned: t('已放弃'),
  succeeded: t('成功'),
});

// JSON syntax highlighting function
const highlightJson = (str) => {
  const escaped = escapeHtml(str);
  return escaped.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g,
    (match) => {
      let color = '#b5cea8'; // numbers
      if (/^"/.test(match)) {
        color = /:$/.test(match) ? '#9cdcfe' : '#ce9178'; // keys vs strings
      } else if (/true|false|null/.test(match)) {
        color = '#569cd6'; // booleans and null
      }
      return `<span style="color: ${color}">${match}</span>`;
    },
  );
};

const JsonViewer = ({
  data,
  t,
  isStream = false,
  label = 'data',
  bodyTruncated = false,
  bodySize = 0,
  requestId = '',
  bodyType = '',
}) => {
  const [wordWrap, setWordWrap] = useState(true);

  // Check if content is too large BEFORE parsing
  // Two thresholds:
  // 1. Frontend display limit: 20,000 bytes - refuse to display inline
  // 2. Backend truncation flag: indicates content was truncated at 1MB
  // Use bodySize (from backend) instead of checking actual data length
  const isLengthExceeded = bodyTruncated || bodySize > BODY_DISPLAY_LIMIT_BYTES;

  const { formatted, highlighted } = useMemo(() => {
    if (!data || isLengthExceeded) return { formatted: '', highlighted: '' };

    let formatted;
    try {
      if (typeof data === 'string') {
        const parsed = JSON.parse(data);
        formatted = JSON.stringify(parsed, null, 2);
      } else {
        formatted = JSON.stringify(data, null, 2);
      }
    } catch {
      formatted = typeof data === 'string' ? data : JSON.stringify(data);
    }

    const highlighted = highlightJson(formatted);

    return { formatted, highlighted };
  }, [data, isLengthExceeded]);

  const handleDownload = useCallback(async () => {
    try {
      let downloadContent = formatted;

      // If content is too large and body is not included, fetch it from API
      if (isLengthExceeded && (!data || data === '') && requestId && bodyType) {
        const response = await API.get(
          `/api/monitor/requests/${requestId}/body/${bodyType}`,
          {
            skipErrorHandler: true,
          },
        );

        if (
          response.data.success &&
          response.data.data &&
          response.data.data.body
        ) {
          downloadContent = response.data.data.body;
        } else {
          throw new Error('Invalid response from server');
        }
      } else if (isLengthExceeded && data) {
        // Body is included but too large to display
        downloadContent =
          typeof data === 'string' ? data : JSON.stringify(data, null, 2);
      }

      const blob = new Blob([downloadContent], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${label}-${Date.now()}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      // TODO: Show error message to user
    }
  }, [formatted, label, isLengthExceeded, data, requestId, bodyType]);

  const handleCopyAll = useCallback(async () => {
    try {
      const content =
        formatted ||
        (typeof data === 'string' ? data : JSON.stringify(data ?? {}, null, 2));
      if (navigator?.clipboard?.writeText) {
        await navigator.clipboard.writeText(content);
      } else {
        const textarea = document.createElement('textarea');
        textarea.value = content;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.focus();
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
      }
    } catch (error) {
      // TODO: Show error message to user
    }
  }, [formatted, data]);

  // Check if content is too large FIRST (before checking !data)
  // This handles the case where backend intentionally excludes body due to size
  if (isLengthExceeded) {
    return (
      <div
        style={{
          background: 'var(--semi-color-fill-0)',
          padding: '16px',
          borderRadius: '6px',
          textAlign: 'center',
        }}
      >
        <Text
          type='tertiary'
          style={{ display: 'block', marginBottom: '12px' }}
        >
          {t('内容长度超出限制')}
        </Text>
        <Button type='primary' size='small' onClick={handleDownload}>
          {t('下载为 JSON 文件')}
        </Button>
      </div>
    );
  }

  if (!data) return <Text type='tertiary'>{t('暂无数据')}</Text>;

  return (
    <div style={{ position: 'relative' }}>
      {isStream && (
        <div
          style={{
            background: 'var(--semi-color-warning-light-default)',
            padding: '8px 12px',
            borderRadius: '6px 6px 0 0',
            marginBottom: '0',
          }}
        >
          <Text size='small' type='tertiary'>
            {t('以下内容为流式响应的拼接汇总，原始内容不可用')}
          </Text>
        </div>
      )}
      <pre
        style={{
          background: '#1e1e1e',
          padding: '12px',
          paddingBottom: '40px',
          borderRadius: isStream ? '0 0 6px 6px' : '6px',
          overflowX: 'auto',
          overflowY: 'visible',
          fontSize: '12px',
          margin: 0,
          whiteSpace: wordWrap ? 'pre-wrap' : 'pre',
          wordBreak: wordWrap ? 'break-all' : 'normal',
          color: '#d4d4d4',
          fontFamily: 'Consolas, "Courier New", Monaco, monospace',
        }}
        dangerouslySetInnerHTML={{ __html: highlighted }}
      />
      <div
        style={{
          position: 'absolute',
          right: '8px',
          top: '8px',
          zIndex: 1,
          display: 'flex',
          gap: '6px',
        }}
      >
        <Tooltip content={t('复制全部')}>
          <Button
            icon={<Copy size={14} />}
            size='small'
            theme='borderless'
            style={{
              backgroundColor: 'rgba(45, 45, 45, 0.9)',
              border: '1px solid rgba(255, 255, 255, 0.1)',
            }}
            onClick={handleCopyAll}
          />
        </Tooltip>
        <Tooltip content={wordWrap ? t('关闭自动换行') : t('自动换行')}>
          <Button
            icon={<WrapText size={14} />}
            size='small'
            theme={wordWrap ? 'solid' : 'borderless'}
            style={{
              backgroundColor: 'rgba(45, 45, 45, 0.9)',
              border: '1px solid rgba(255, 255, 255, 0.1)',
            }}
            onClick={() => setWordWrap(!wordWrap)}
          />
        </Tooltip>
      </div>
    </div>
  );
};

const HeadersViewer = ({ headers, t }) => {
  if (!headers || Object.keys(headers).length === 0) {
    return <Text type='tertiary'>{t('无请求头')}</Text>;
  }

  return (
    <div
      style={{
        background: 'var(--semi-color-fill-0)',
        padding: '12px',
        borderRadius: '6px',
      }}
    >
      {Object.entries(headers).map(([key, value]) => (
        <div key={key} style={{ marginBottom: '4px' }}>
          <Text strong style={{ color: 'var(--semi-color-primary)' }}>
            {key}:
          </Text>{' '}
          <Text>{value}</Text>
        </div>
      ))}
    </div>
  );
};

const DetailCardTitle = ({ icon, text }) => (
  <Space spacing='tight'>
    <span
      style={{ display: 'inline-flex', color: 'var(--semi-color-primary)' }}
    >
      {icon}
    </span>
    <span>{text}</span>
  </Space>
);

const MetaPill = ({ icon, label, children, style }) => (
  <div
    style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: '6px',
      minWidth: 0,
      maxWidth: '100%',
      flex: '1 1 280px',
      padding: '4px 10px',
      borderRadius: '999px',
      background: 'var(--semi-color-fill-0)',
      ...style,
    }}
  >
    <span
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        color: 'var(--semi-color-text-2)',
        flexShrink: 0,
      }}
    >
      {icon}
    </span>
    <Text type='tertiary' size='small' style={{ flexShrink: 0 }}>
      {label}
    </Text>
    <span style={{ display: 'inline-flex', alignItems: 'center', minWidth: 0 }}>
      {children}
    </span>
  </div>
);

const DetailPanelHeader = ({ label, onToggle, headerRef }) => (
  <div
    ref={headerRef}
    role='button'
    tabIndex={0}
    onMouseDown={(event) => event.preventDefault()}
    onClick={(event) => {
      event.preventDefault();
      event.stopPropagation();
      onToggle();
    }}
    onKeyDown={(event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        event.stopPropagation();
        onToggle();
      }
    }}
    style={{
      userSelect: 'none',
      display: 'inline-flex',
      alignItems: 'center',
      minHeight: '22px',
    }}
  >
    {label}
  </div>
);

const detailStateContainerStyle = {
  display: 'grid',
  placeItems: 'center',
  width: '100%',
  height: '100%',
  minHeight: '400px',
  textAlign: 'center',
};

const RequestDetail = ({
  record,
  loading,
  error,
  t,
  statusLabels,
  onInterrupt,
  interrupting,
  scrollContainerRef,
  visible,
}) => {
  const phaseLabels = useMemo(() => getPhaseLabels(t), [t]);
  const attemptLabels = useMemo(() => getAttemptStatusLabels(t), [t]);
  const displayStatus = useMemo(() => deriveDisplayStatus(record), [record]);
  const [interruptError, setInterruptError] = useState(null);
  const [activeDetailPanelKey, setActiveDetailPanelKey] = useState('');
  const interruptErrorTimeoutRef = useRef(null);
  const scrollAnimationFrameRef = useRef(null);
  const shouldAutoExpandResponseBodyRef = useRef(false);
  const skipNextEnsureVisibleRef = useRef(false);
  const detailPanelHeaderRefs = useRef({});
  const stopwatch = useStopwatch(record, t);

  useEffect(() => {
    setActiveDetailPanelKey('');
    shouldAutoExpandResponseBodyRef.current = visible;
  }, [record?.id]);

  useEffect(() => {
    if (visible) {
      shouldAutoExpandResponseBodyRef.current = true;
      return;
    }
    shouldAutoExpandResponseBodyRef.current = false;
  }, [visible]);

  useEffect(() => {
    if (!visible || !record?.id || !record?.response) return;
    if (!shouldAutoExpandResponseBodyRef.current) return;

    setActiveDetailPanelKey((previousKey) => {
      if (previousKey === 'response-body') return previousKey;
      skipNextEnsureVisibleRef.current = true;
      return 'response-body';
    });
    shouldAutoExpandResponseBodyRef.current = false;
  }, [visible, record?.id, record?.response]);

  const handleDetailPanelToggle = useCallback((targetKey) => {
    setActiveDetailPanelKey((previousKey) =>
      previousKey === targetKey ? '' : targetKey,
    );
  }, []);

  const registerDetailPanelHeaderRef = useCallback((panelKey, node) => {
    if (node) {
      detailPanelHeaderRefs.current[panelKey] = node;
      return;
    }
    delete detailPanelHeaderRefs.current[panelKey];
  }, []);

  const handleDetailPanelChange = useCallback((nextKey) => {
    const normalizedKeys = (
      Array.isArray(nextKey) ? nextKey : [nextKey]
    ).filter((key) => typeof key === 'string' && key.length > 0);

    if (normalizedKeys.length === 0) {
      setActiveDetailPanelKey('');
      return;
    }

    setActiveDetailPanelKey(
      (previousKey) =>
        normalizedKeys.find((key) => key !== previousKey) ||
        normalizedKeys[normalizedKeys.length - 1],
    );
  }, []);

  const downstreamDetailActiveKey = activeDetailPanelKey?.startsWith(
    'downstream-',
  )
    ? [activeDetailPanelKey]
    : [];
  const responseDetailActiveKey = activeDetailPanelKey?.startsWith('response-')
    ? [activeDetailPanelKey]
    : [];

  const smoothScrollTo = useCallback((container, targetTop) => {
    const maxScrollTop = Math.max(
      0,
      container.scrollHeight - container.clientHeight,
    );
    const clampedTargetTop = Math.max(0, Math.min(targetTop, maxScrollTop));
    const startTop = container.scrollTop;
    const delta = clampedTargetTop - startTop;

    if (Math.abs(delta) < 1) return;

    const prefersReducedMotion =
      typeof window !== 'undefined' &&
      window.matchMedia &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    if (prefersReducedMotion) {
      container.scrollTop = clampedTargetTop;
      return;
    }

    if (scrollAnimationFrameRef.current) {
      cancelAnimationFrame(scrollAnimationFrameRef.current);
      scrollAnimationFrameRef.current = null;
    }

    const durationMs = 280;
    const startTime = performance.now();
    const easeInOutCubic = (progress) =>
      progress < 0.5
        ? 4 * progress * progress * progress
        : 1 - Math.pow(-2 * progress + 2, 3) / 2;

    const animate = (now) => {
      const elapsed = now - startTime;
      const progress = Math.min(1, elapsed / durationMs);
      container.scrollTop = startTop + delta * easeInOutCubic(progress);

      if (progress < 1) {
        scrollAnimationFrameRef.current = requestAnimationFrame(animate);
      } else {
        scrollAnimationFrameRef.current = null;
      }
    };

    scrollAnimationFrameRef.current = requestAnimationFrame(animate);
  }, []);

  const ensureExpandedPanelVisible = useCallback(
    (panelKey) => {
      if (!panelKey) return;

      const scrollContainer = scrollContainerRef?.current;
      const headerElement = detailPanelHeaderRefs.current[panelKey];
      if (!scrollContainer || !headerElement) return;

      const collapseItem = headerElement.closest('.semi-collapse-item');
      if (!collapseItem) return;

      const padding = 8;
      const containerRect = scrollContainer.getBoundingClientRect();
      const panelRect = collapseItem.getBoundingClientRect();
      const isPanelTooTall =
        panelRect.height > containerRect.height - padding * 2;
      const isPanelBottomClipped =
        panelRect.bottom > containerRect.bottom - padding;

      if (!isPanelTooTall && !isPanelBottomClipped) return;

      const headerRect = headerElement.getBoundingClientRect();
      const targetScrollTop =
        scrollContainer.scrollTop +
        (headerRect.top - containerRect.top) -
        padding;

      smoothScrollTo(scrollContainer, targetScrollTop);
    },
    [scrollContainerRef, smoothScrollTo],
  );

  useEffect(() => {
    if (!activeDetailPanelKey) return;
    if (skipNextEnsureVisibleRef.current) {
      skipNextEnsureVisibleRef.current = false;
      return;
    }

    const timerId = setTimeout(() => {
      ensureExpandedPanelVisible(activeDetailPanelKey);
    }, 180);

    return () => clearTimeout(timerId);
  }, [activeDetailPanelKey, ensureExpandedPanelVisible]);

  useEffect(() => {
    return () => {
      if (interruptErrorTimeoutRef.current) {
        clearTimeout(interruptErrorTimeoutRef.current);
        interruptErrorTimeoutRef.current = null;
      }

      if (scrollAnimationFrameRef.current) {
        cancelAnimationFrame(scrollAnimationFrameRef.current);
        scrollAnimationFrameRef.current = null;
      }
    };
  }, []);

  // Check if request is active (can be interrupted)
  const isActive = useMemo(() => {
    if (!record) return false;
    return isActiveStatus(displayStatus);
  }, [record, displayStatus]);

  // Find the currently active attempt (last attempt with active status)
  const activeAttemptIndex = useMemo(() => {
    if (!record?.channel_attempts || record.channel_attempts.length === 0)
      return -1;

    // Find the last attempt that is in an active state
    for (let i = record.channel_attempts.length - 1; i >= 0; i--) {
      const attempt = record.channel_attempts[i];
      if (
        attempt.status === 'waiting_upstream' ||
        attempt.status === 'streaming'
      ) {
        return i;
      }
    }
    return -1;
  }, [record]);

  const handleInterrupt = async () => {
    if (!record?.id) return;

    setInterruptError(null);
    const result = await onInterrupt(record.id);

    if (!result.success) {
      setInterruptError(result.error);
      // Clear error after 5 seconds
      if (interruptErrorTimeoutRef.current) {
        clearTimeout(interruptErrorTimeoutRef.current);
      }
      interruptErrorTimeoutRef.current = setTimeout(() => {
        setInterruptError(null);
        interruptErrorTimeoutRef.current = null;
      }, 5000);
    }
  };

  // Loading state
  if (loading) {
    return (
      <div style={detailStateContainerStyle}>
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <Spin size='large' />
          <Text type='tertiary' style={{ whiteSpace: 'nowrap' }}>
            {t('正在加载请求详情...')}
          </Text>
        </div>
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div style={detailStateContainerStyle}>
        <Empty description={t('错误: {{message}}', { message: error })} />
      </div>
    );
  }

  // No selection state
  if (!record) {
    return (
      <div style={detailStateContainerStyle}>
        <Empty description={t('选择一个请求查看详情')} />
      </div>
    );
  }

  return (
    <div style={{ padding: '4px 6px' }}>
      <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
        <Card
          title={
            <DetailCardTitle
              icon={<Network size={15} />}
              text={t('当前渠道 / 重试状态')}
            />
          }
          style={{ width: '100%' }}
          bodyStyle={{ padding: '10px 12px' }}
        >
          <Space
            vertical
            align='start'
            style={{ width: '100%' }}
            spacing='small'
          >
            <div
              style={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: '8px',
                width: '100%',
              }}
            >
              <MetaPill icon={<Route size={14} />} label={t('当前渠道')}>
                {record.current_channel ? (
                  <Tag color='blue' size='small'>
                    {record.current_channel.name || '-'}
                    {' / '}
                    ID {record.current_channel.id || '-'}
                    {' / '}
                    {t('第 {{num}} 次', {
                      num: record.current_channel.attempt || 1,
                    })}
                  </Tag>
                ) : (
                  <Text type='tertiary' size='small'>
                    {t('暂未选择渠道')}
                  </Text>
                )}
              </MetaPill>

              <MetaPill icon={<Activity size={14} />} label={t('当前响应状态')}>
                <Tag
                  color={record.status === 'abandoned' ? 'grey' : (channelPhaseColors[record.current_phase] || 'grey')}
                  size='small'
                >
                  {record.status === 'abandoned' ? t('放弃') : (phaseLabels[record.current_phase] || t('未知状态'))}
                </Tag>
              </MetaPill>

              {stopwatch.isActive && (
                <MetaPill icon={<Clock3 size={14} />} label={t('计时')}>
                  <Text
                    size='small'
                    style={{
                      fontFamily:
                        '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans", sans-serif, "PingFang SC", "Microsoft YaHei"',
                      color: 'var(--semi-color-text-1)',
                    }}
                  >
                    {stopwatch.display}
                  </Text>
                </MetaPill>
              )}
            </div>

            <div
              style={{
                width: '100%',
                marginTop: '4px',
                paddingTop: '10px',
                borderTop: '1px solid var(--semi-color-border)',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  flexWrap: 'wrap',
                  gap: '8px',
                  marginBottom: '10px',
                }}
              >
                <Space spacing='tight'>
                  <History
                    size={14}
                    style={{ color: 'var(--semi-color-text-2)' }}
                  />
                  <Text strong>{t('渠道重试历史')}</Text>
                </Space>
                {(record.channel_attempts || []).length > 0 && (
                  <Tag size='small' color='grey'>
                    {(record.channel_attempts || []).length}
                  </Tag>
                )}
              </div>

              {(record.channel_attempts || []).length === 0 ? (
                <Text type='tertiary'>{t('暂无渠道重试记录')}</Text>
              ) : (
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '8px',
                    width: '100%',
                  }}
                >
                  {(record.channel_attempts || []).map((attempt, index) => {
                    const isActiveAttempt = index === activeAttemptIndex;
                    return (
                      <div
                        key={`${attempt.attempt}-${attempt.channel_id}-${attempt.started_at}`}
                        style={{
                          width: '100%',
                          padding: '8px 10px',
                          borderRadius: '8px',
                          background: 'var(--semi-color-fill-0)',
                        }}
                      >
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            flexWrap: 'wrap',
                            gap: '6px 10px',
                          }}
                        >
                          <div
                            style={{
                              display: 'flex',
                              alignItems: 'center',
                              flexWrap: 'wrap',
                              gap: '6px 10px',
                            }}
                          >
                            <Tag size='small'>
                              {t('第 {{num}} 次', { num: attempt.attempt })}
                            </Tag>
                            <Text size='small'>
                              {attempt.channel_name || t('未知渠道')} (ID:{' '}
                              {attempt.channel_id || '-'})
                            </Text>
                            <Tag
                              color={
                                attemptStatusColors[attempt.status] || 'grey'
                              }
                              size='small'
                            >
                              {attemptLabels[attempt.status] ||
                                attempt.status ||
                                t('未知状态')}
                            </Tag>
                          </div>
                          {isActive && isActiveAttempt && (
                            <Tooltip
                              content={t('中断当前请求并尝试下一个渠道')}
                            >
                              <Button
                                type='danger'
                                size='small'
                                loading={interrupting}
                                disabled={interrupting}
                                onClick={handleInterrupt}
                              >
                                {t('中断')}
                              </Button>
                            </Tooltip>
                          )}
                        </div>

                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            flexWrap: 'wrap',
                            gap: '6px 12px',
                            marginTop: '4px',
                          }}
                        >
                          <Text type='tertiary' size='small'>
                            {t('开始')}:{' '}
                            {attempt.started_at
                              ? new Date(
                                attempt.started_at,
                              ).toLocaleTimeString()
                              : '-'}
                            {attempt.ended_at
                              ? ` | ${t('结束')}: ${new Date(attempt.ended_at).toLocaleTimeString()}`
                              : ''}
                          </Text>
                          {(attempt.reason ||
                            attempt.error_code ||
                            attempt.http_status) && (
                              <Text type='tertiary' size='small'>
                                {t('原因')}: {attempt.reason || '-'}
                                {attempt.error_code
                                  ? ` | ${t('错误码')}: ${attempt.error_code}`
                                  : ''}
                                {attempt.http_status
                                  ? ` | HTTP ${attempt.http_status}`
                                  : ''}
                              </Text>
                            )}
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
            {interruptError && (
              <div
                style={{
                  marginTop: '6px',
                  padding: '8px 10px',
                  background: 'var(--semi-color-danger-light-default)',
                  borderRadius: '6px',
                }}
              >
                <Text type='danger' size='small'>
                  {t('中断失败')}: {interruptError}
                </Text>
              </div>
            )}
          </Space>
        </Card>

        <Card
          title={
            <DetailCardTitle icon={<Hash size={15} />} text={t('请求信息')} />
          }
          style={{ width: '100%' }}
          bodyStyle={{ padding: '10px 12px' }}
        >
          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: '8px',
              width: '100%',
              marginBottom: '8px',
            }}
          >
            <MetaPill
              icon={<Hash size={14} />}
              label={t('请求 ID')}
              style={{ flex: '2 1 460px' }}
            >
              <Text size='small' style={{ wordBreak: 'break-all' }}>
                {record.id}
              </Text>
            </MetaPill>
            <MetaPill icon={<Route size={14} />} label={t('渠道')}>
              {record.channel_name ? (
                <Tag
                  color={stringToColor(
                    record.channel_name || String(record.channel_id || ''),
                  )}
                  shape='circle'
                  size='small'
                >
                  {record.channel_name}
                </Tag>
              ) : (
                <Text type='tertiary' size='small'>
                  -
                </Text>
              )}
            </MetaPill>
            <MetaPill icon={<Network size={14} />} label={t('模型')}>
              {record.model ? (
                renderModelTag(record.model, { shape: 'circle', size: 'small' })
              ) : (
                <Text type='tertiary' size='small'>
                  -
                </Text>
              )}
            </MetaPill>
          </div>

          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: '8px',
              width: '100%',
            }}
          >
            <MetaPill icon={<Activity size={14} />} label={t('状态')}>
              <Tag
                color={
                  statusColors[displayStatus] ||
                  statusColors[record.status] ||
                  'grey'
                }
                size='small'
              >
                {statusLabels[displayStatus] ||
                  statusLabels[record.status] ||
                  displayStatus ||
                  record.status ||
                  t('未知状态')}
              </Tag>
            </MetaPill>
            <MetaPill icon={<Radio size={14} />} label={t('是否流式')}>
              {record.is_stream ? (
                <Tag color='blue' size='small'>
                  {t('是')}
                </Tag>
              ) : (
                <Tag size='small'>{t('否')}</Tag>
              )}
            </MetaPill>
            <MetaPill icon={<Clock3 size={14} />} label={t('开始时间')}>
              <Text size='small'>
                {formatMonthDayTime(record.start_time_ms, record.start_time)}
              </Text>
            </MetaPill>
            <MetaPill icon={<Clock3 size={14} />} label={t('耗时')}>
              <DurationCell record={record} t={t} />
            </MetaPill>
            <MetaPill icon={<User size={14} />} label={t('用户 ID')}>
              <Text size='small'>{record.user_id || '-'}</Text>
            </MetaPill>
            <MetaPill icon={<KeyRound size={14} />} label={t('令牌')}>
              <Text
                size='small'
                style={{ maxWidth: 220 }}
                ellipsis={{ showTooltip: true }}
              >
                {record.token_name || '-'}
              </Text>
            </MetaPill>
          </div>
        </Card>

        <Card
          title={
            <DetailCardTitle
              icon={<ArrowDownToLine size={15} />}
              text={t('下游请求（客户端）')}
            />
          }
          style={{ width: '100%' }}
          bodyStyle={{ padding: '10px 12px' }}
        >
          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              gap: '8px',
              width: '100%',
              marginBottom: '8px',
            }}
          >
            <MetaPill icon={<Route size={14} />} label={t('请求路径')}>
              <Text
                size='small'
                style={{ maxWidth: 460 }}
                ellipsis={{ showTooltip: true }}
              >
                {record.downstream?.method || '-'}{' '}
                {record.downstream?.path || '-'}
              </Text>
            </MetaPill>
            <MetaPill icon={<Globe2 size={14} />} label={t('客户端 IP')}>
              <Text size='small'>{record.downstream?.client_ip || '-'}</Text>
            </MetaPill>
            {record.downstream?.body_size > 0 && (
              <MetaPill icon={<Hash size={14} />} label={t('请求体大小')}>
                <Text size='small'>
                  {t('{{size}} 字节', { size: record.downstream.body_size })}
                </Text>
              </MetaPill>
            )}
          </div>

          <Collapse
            accordion={false}
            activeKey={downstreamDetailActiveKey}
            onChange={handleDetailPanelChange}
          >
            <Collapse.Panel
              header={
                <DetailPanelHeader
                  label={t('请求头')}
                  onToggle={() => handleDetailPanelToggle('downstream-headers')}
                  headerRef={(node) =>
                    registerDetailPanelHeaderRef('downstream-headers', node)
                  }
                />
              }
              itemKey='downstream-headers'
            >
              <HeadersViewer headers={record.downstream?.headers} t={t} />
            </Collapse.Panel>
            <Collapse.Panel
              header={
                <DetailPanelHeader
                  label={t('请求体')}
                  onToggle={() => handleDetailPanelToggle('downstream-body')}
                  headerRef={(node) =>
                    registerDetailPanelHeaderRef('downstream-body', node)
                  }
                />
              }
              itemKey='downstream-body'
            >
              <JsonViewer
                data={record.downstream?.body}
                t={t}
                isStream={false}
                label='downstream-request-body'
                bodyTruncated={record.downstream?.body_truncated}
                bodySize={record.downstream?.body_size || 0}
                requestId={record.id}
                bodyType='downstream'
              />
            </Collapse.Panel>
          </Collapse>
        </Card>

        {record.response && (
          <Card
            title={
              <DetailCardTitle
                icon={<ArrowUpFromLine size={15} />}
                text={t('响应')}
              />
            }
            style={{ width: '100%' }}
            bodyStyle={{ padding: '10px 12px' }}
          >
            <div
              style={{
                display: 'flex',
                flexWrap: 'wrap',
                gap: '8px',
                width: '100%',
              }}
            >
              <MetaPill icon={<Hash size={14} />} label={t('状态码')}>
                <Tag
                  color={record.response.status_code >= 400 ? 'red' : 'green'}
                  size='small'
                >
                  {record.response.status_code}
                </Tag>
              </MetaPill>
              <MetaPill
                icon={<ArrowDownToLine size={14} />}
                label={t('提示词 Tokens')}
              >
                <Text size='small'>{record.response.prompt_tokens || 0}</Text>
              </MetaPill>
              <MetaPill
                icon={<ArrowUpFromLine size={14} />}
                label={t('补全 Tokens')}
              >
                <Text size='small'>
                  {record.response.completion_tokens || 0}
                </Text>
              </MetaPill>
            </div>

            {record.response.error && record.status !== 'abandoned' && (
              <div
                style={{
                  marginTop: '10px',
                  padding: '10px',
                  background: 'var(--semi-color-danger-light-default)',
                  borderRadius: '6px',
                }}
              >
                <Text type='danger' strong>
                  {t('错误: {{message}}', {
                    message: record.response.error.message,
                  })}
                </Text>
              </div>
            )}

            <Collapse
              accordion={false}
              activeKey={responseDetailActiveKey}
              onChange={handleDetailPanelChange}
              style={{ marginTop: '10px' }}
            >
              <Collapse.Panel
                header={
                  <DetailPanelHeader
                    label={t('响应头')}
                    onToggle={() => handleDetailPanelToggle('response-headers')}
                    headerRef={(node) =>
                      registerDetailPanelHeaderRef('response-headers', node)
                    }
                  />
                }
                itemKey='response-headers'
              >
                <HeadersViewer headers={record.response?.headers} t={t} />
              </Collapse.Panel>
              <Collapse.Panel
                header={
                  <DetailPanelHeader
                    label={t('响应体')}
                    onToggle={() => handleDetailPanelToggle('response-body')}
                    headerRef={(node) =>
                      registerDetailPanelHeaderRef('response-body', node)
                    }
                  />
                }
                itemKey='response-body'
              >
                <JsonViewer
                  data={record.response?.body}
                  t={t}
                  isStream={record.is_stream}
                  label='upstream-response-body'
                  bodyTruncated={record.response?.body_truncated}
                  bodySize={record.response?.body_size || 0}
                  requestId={record.id}
                  bodyType='response'
                />
              </Collapse.Panel>
            </Collapse>
          </Card>
        )}
      </Space>
    </div>
  );
};

const Monitor = () => {
  const { t } = useTranslation();
  const [selectedId, setSelectedId] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [filter, setFilter] = useState('all');
  const [isCompact, setIsCompact] = useState(false);
  const tableRef = useRef(null);
  const detailScrollContainerRef = useRef(null);
  // Track previous status to detect status changes
  const prevStatusRef = useRef(new Map());

  const { summaries, stats, connected, reconnect, channelUpdate } =
    useMonitorWs({
      focusedRequestId: selectedId,
    });
  const {
    selectedDetail,
    loading: detailLoading,
    error: detailError,
    interrupting,
    fetchDetail,
    invalidateCache,
    clearCache,
    applyLiveUpdate,
    interruptRequest,
  } = useRequestDetail();

  const statusLabels = useMemo(() => getStatusLabels(t), [t]);

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) {
      return;
    }

    const mediaQuery = window.matchMedia('(max-width: 1400px)');
    const updateCompactMode = (event) => {
      setIsCompact(event.matches);
    };

    setIsCompact(mediaQuery.matches);

    if (mediaQuery.addEventListener) {
      mediaQuery.addEventListener('change', updateCompactMode);
      return () => mediaQuery.removeEventListener('change', updateCompactMode);
    }

    mediaQuery.addListener(updateCompactMode);
    return () => mediaQuery.removeListener(updateCompactMode);
  }, []);

  const summariesWithStatus = useMemo(() => {
    return summaries.map((summary) => {
      const startTimeMs = getTimestampMs(
        summary.start_time_ms,
        summary.start_time,
      );
      const displayStatus = deriveDisplayStatus(summary);
      return {
        ...summary,
        startTimeMs,
        displayStatus,
      };
    });
  }, [summaries]);

  // Fetch detail when selection changes
  useEffect(() => {
    if (selectedId) {
      fetchDetail(selectedId);
    }
  }, [selectedId, fetchDetail]);

  // Apply live channel updates streamed over WebSocket
  useEffect(() => {
    if (!selectedId || !channelUpdate) return;

    const updateId = channelUpdate.request_id || channelUpdate.id;
    if (updateId === selectedId) {
      applyLiveUpdate(selectedId, channelUpdate);
    }
  }, [selectedId, channelUpdate, applyLiveUpdate]);

  // Invalidate cache when a request's status changes (e.g., processing -> completed)
  // This ensures we fetch fresh data with response details
  useEffect(() => {
    const visibleIds = new Set();
    summariesWithStatus.forEach((summary) => {
      visibleIds.add(summary.id);
      const prevStatus = prevStatusRef.current.get(summary.id);
      const displayStatus =
        summary.displayStatus || deriveDisplayStatus(summary);
      const statusChanged = prevStatus && prevStatus !== displayStatus;

      if (statusChanged && isTerminalStatus(displayStatus)) {
        // Status changed, invalidate cache to get fresh data
        invalidateCache(summary.id);

        // If this is the currently selected item, refetch
        if (selectedId === summary.id) {
          fetchDetail(summary.id);
        }
      }
      prevStatusRef.current.set(summary.id, displayStatus);
    });

    // Prevent unbounded growth: keep status history only for rows we still render.
    for (const id of prevStatusRef.current.keys()) {
      if (!visibleIds.has(id)) {
        prevStatusRef.current.delete(id);
      }
    }
  }, [summaries, selectedId, invalidateCache, fetchDetail]);

  // Clear cache on reconnect
  useEffect(() => {
    if (!connected) {
      clearCache({ preserveSelection: true });
      prevStatusRef.current.clear();
    }
  }, [connected, clearCache]);

  // If the selected request is evicted from the summaries buffer, clear selection
  useEffect(() => {
    if (!selectedId) return;
    const stillExists = summariesWithStatus.some(
      (summary) => summary.id === selectedId,
    );
    if (!stillExists) {
      setSelectedId(null);
      setDetailVisible(false);
      clearCache();
    }
  }, [summariesWithStatus, selectedId, clearCache]);

  // Refresh detail after reconnect to keep the selection visible and updated
  useEffect(() => {
    if (connected && selectedId) {
      fetchDetail(selectedId);
    }
  }, [connected, selectedId, fetchDetail]);

  const handleRowClick = useCallback((record) => {
    setSelectedId(record.id);
    setDetailVisible(true);
  }, []);

  const localActiveCount = useMemo(() => {
    return summariesWithStatus.filter((r) =>
      isActiveStatus(r.displayStatus || deriveDisplayStatus(r)),
    ).length;
  }, [summariesWithStatus]);

  const filteredSummaries = useMemo(() => {
    return summariesWithStatus.filter((r) => {
      const displayStatus = r.displayStatus || deriveDisplayStatus(r);
      if (filter === 'all') return true;
      if (filter === 'processing') return isActiveStatus(displayStatus);
      return displayStatus === filter;
    });
  }, [summariesWithStatus, filter]);

  // Sort by start_time descending (newest first)
  const sortedSummaries = useMemo(() => {
    return [...filteredSummaries].sort((a, b) => {
      return (b.startTimeMs || 0) - (a.startTimeMs || 0);
    });
  }, [filteredSummaries]);

  const columns = useMemo(() => {
    return [
      {
        title: t('时间'),
        dataIndex: 'start_time',
        width: isCompact ? 126 : 146,
        render: (time, record) => {
          if (!time) return '-';
          return formatMonthDayTime(record?.start_time_ms, record?.start_time);
        },
      },
      {
        title: t('状态'),
        dataIndex: 'status',
        width: isCompact ? 86 : 98,
        render: (_, record) => {
          const displayStatus =
            record.displayStatus || deriveDisplayStatus(record);
          return (
            <Tag
              color={
                statusColors[displayStatus] ||
                statusColors[record.status] ||
                'grey'
              }
            >
              {statusLabels[displayStatus] ||
                statusLabels[record.status] ||
                displayStatus ||
                record.status}
            </Tag>
          );
        },
      },
      {
        title: t('模型'),
        dataIndex: 'model',
        width: isCompact ? undefined : 180,
        ellipsis: true,
        render: (_, record) =>
          renderModelTag(record.model || t('未知模型'), {
            shape: 'circle',
          }),
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_name',
        width: isCompact ? undefined : 160,
        ellipsis: true,
        render: (_, record) => (
          <Tag
            color={stringToColor(
              record.channel_name || String(record.channel_id || ''),
            )}
            shape='circle'
          >
            {record.channel_name || t('未知渠道')}
          </Tag>
        ),
      },
      {
        title: t('耗时'),
        dataIndex: 'duration_ms',
        width: isCompact ? 76 : 90,
        render: (_, record) => <DurationCell record={record} t={t} />,
      },
    ];
  }, [t, statusLabels, isCompact]);

  return (
    <div className='mt-[60px] px-2'>
      <Card className='table-scroll-card'>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '12px',
            flexShrink: 0,
          }}
        >
          <Space>
            <Title heading={4} style={{ margin: 0 }}>
              {t('请求监控')}
            </Title>
            <Badge
              count={connected ? t('已连接') : t('已断开')}
              type={connected ? 'success' : 'danger'}
            />
          </Space>
          <Space>
            {!connected && (
              <Button icon={<IconRefresh />} onClick={reconnect}>
                {t('重新连接')}
              </Button>
            )}
          </Space>
        </div>

        {/* Filter Tabs */}
        <Tabs
          type='button'
          activeKey={filter}
          onChange={setFilter}
          style={{ marginBottom: '12px', flexShrink: 0 }}
        >
          <TabPane tab={t('全部')} itemKey='all' />
          <TabPane
            tab={
              <Space>
                {t('处理中')}
                <Badge count={localActiveCount} type='primary' />
              </Space>
            }
            itemKey='processing'
          />
          <TabPane tab={t('完成')} itemKey='completed' />
          <TabPane tab={t('错误')} itemKey='error' />
          <TabPane tab={t('放弃')} itemKey='abandoned' />
        </Tabs>

        <div style={{ flex: 1, minHeight: 0 }}>
          <Table
            ref={tableRef}
            columns={columns}
            dataSource={sortedSummaries}
            rowKey='id'
            pagination={false}
            size='small'
            onRow={(record) => ({
              onClick: () => handleRowClick(record),
              style: {
                cursor: 'pointer',
                background:
                  selectedId === record.id
                    ? 'var(--semi-color-primary-light-default)'
                    : undefined,
              },
            })}
            empty={
              <Empty
                description={connected ? t('暂无请求') : t('正在连接服务器...')}
              />
            }
          />
        </div>
      </Card>

      <Modal
        title={t('请求详情')}
        visible={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={'92vw'}
        centered
        bodyStyle={{ height: '92vh', padding: 0, overflow: 'hidden' }}
      >
        <div
          ref={detailScrollContainerRef}
          style={{
            height: '100%',
            overflow: 'auto',
            padding: '6px 8px',
          }}
        >
          <RequestDetail
            record={selectedDetail}
            loading={detailLoading}
            error={detailError}
            t={t}
            statusLabels={statusLabels}
            onInterrupt={interruptRequest}
            interrupting={interrupting}
            scrollContainerRef={detailScrollContainerRef}
            visible={detailVisible}
          />
        </div>
      </Modal>
    </div>
  );
};

export default Monitor;
