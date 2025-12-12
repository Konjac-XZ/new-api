import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import {
  Card,
  Table,
  Tag,
  Typography,
  Space,
  Button,
  Descriptions,
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
import { WrapText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import useMonitorWs from './useMonitorWs';
import useRequestDetail from './useRequestDetail';
import { useStopwatch } from './useStopwatch';
import { deriveDisplayStatus, isActiveStatus, isTerminalStatus } from './statusUtils';
import { renderModelTag, stringToColor } from '../../helpers';

const { Title, Text } = Typography;

const statusColors = {
  pending: 'grey',
  processing: 'blue',
  waiting_upstream: 'blue',
  streaming: 'purple',
  completed: 'green',
  error: 'red',
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

const renderDurationTag = (durationMs, t) => {
  if (!durationMs) return <Text type='tertiary'>-</Text>;
  const seconds = Number(durationMs / 1000).toFixed(1);
  const value = parseFloat(seconds);
  let color = 'green';
  if (value >= 10) {
    color = 'red';
  } else if (value >= 3) {
    color = 'orange';
  }
  return (
    <Tag color={color} shape='circle'>
      {seconds}s
    </Tag>
  );
};

// Component to display duration with real-time stopwatch for ongoing requests
const DurationCell = ({ record, t }) => {
  const [elapsed, setElapsed] = useState(0);
  const displayStatus = useMemo(() => deriveDisplayStatus(record), [record]);
  const isActive = useMemo(() => isActiveStatus(displayStatus), [displayStatus]);

  useEffect(() => {
    if (!isActive || !record.start_time) {
      return;
    }

    const updateElapsed = () => {
      const now = Date.now();
      const startTime = new Date(record.start_time).getTime();
      const elapsedSeconds = (now - startTime) / 1000;
      setElapsed(elapsedSeconds);
    };

    updateElapsed();
    const interval = setInterval(updateElapsed, 100);

    return () => clearInterval(interval);
  }, [isActive, record.start_time]);

  if (isActive) {
    return (
      <Tag color='grey' shape='circle'>
        {elapsed.toFixed(1)}s
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
  completed: t('已完成'),
  error: t('错误'),
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
  return str.replace(
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

const JsonViewer = ({ data, t }) => {
  const [wordWrap, setWordWrap] = useState(false);

  const highlighted = useMemo(() => {
    if (!data) return '';

    let formatted;
    try {
      if (typeof data === 'string') {
        const parsed = JSON.parse(data);
        formatted = JSON.stringify(parsed, null, 2);
      } else {
        formatted = JSON.stringify(data, null, 2);
      }
    } catch {
      formatted = data;
    }

    return highlightJson(formatted);
  }, [data]);

  if (!data) return <Text type="tertiary">{t('暂无数据')}</Text>;

  return (
    <div style={{ position: 'relative' }}>
      <pre
        style={{
          background: '#1e1e1e',
          padding: '12px',
          paddingBottom: '40px',
          borderRadius: '6px',
          overflow: 'auto',
          maxHeight: '300px',
          fontSize: '12px',
          margin: 0,
          whiteSpace: wordWrap ? 'pre-wrap' : 'pre',
          wordBreak: wordWrap ? 'break-all' : 'normal',
          color: '#d4d4d4',
          fontFamily: 'Consolas, "Courier New", Monaco, monospace',
        }}
        dangerouslySetInnerHTML={{ __html: highlighted }}
      />
      <Tooltip content={wordWrap ? t('关闭自动换行') : t('自动换行')}>
        <Button
          icon={<WrapText size={14} />}
          size="small"
          theme={wordWrap ? 'solid' : 'borderless'}
          style={{
            position: 'absolute',
            bottom: '8px',
            right: '8px',
            zIndex: 1,
            backgroundColor: 'rgba(45, 45, 45, 0.9)',
            border: '1px solid rgba(255, 255, 255, 0.1)',
          }}
          onClick={() => setWordWrap(!wordWrap)}
        />
      </Tooltip>
    </div>
  );
};

const HeadersViewer = ({ headers, t }) => {
  if (!headers || Object.keys(headers).length === 0) {
    return <Text type="tertiary">{t('无请求头')}</Text>;
  }

  return (
    <div
      style={{
        background: 'var(--semi-color-fill-0)',
        padding: '12px',
        borderRadius: '6px',
        maxHeight: '200px',
        overflow: 'auto',
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

const RequestDetail = ({ record, loading, error, t, statusLabels, onInterrupt, interrupting }) => {
  const phaseLabels = useMemo(() => getPhaseLabels(t), [t]);
  const attemptLabels = useMemo(() => getAttemptStatusLabels(t), [t]);
  const displayStatus = useMemo(() => deriveDisplayStatus(record), [record]);
  const [interruptError, setInterruptError] = useState(null);
  const stopwatch = useStopwatch(record);

  // Check if request is active (can be interrupted)
  const isActive = useMemo(() => {
    if (!record) return false;
    return isActiveStatus(displayStatus);
  }, [record, displayStatus]);

  // Find the currently active attempt (last attempt with active status)
  const activeAttemptIndex = useMemo(() => {
    if (!record?.channel_attempts || record.channel_attempts.length === 0) return -1;

    // Find the last attempt that is in an active state
    for (let i = record.channel_attempts.length - 1; i >= 0; i--) {
      const attempt = record.channel_attempts[i];
      if (attempt.status === 'waiting_upstream' || attempt.status === 'streaming') {
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
      setTimeout(() => setInterruptError(null), 5000);
    }
  };

  // Loading state
  if (loading) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          minHeight: '400px',
        }}
      >
        <Spin size='large' tip={t('正在加载请求详情...')} />
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          minHeight: '400px',
        }}
      >
        <Empty description={t('错误: {{message}}', { message: error })} />
      </div>
    );
  }

  // No selection state
  if (!record) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          minHeight: '400px',
        }}
      >
        <Empty description={t('选择一个请求查看详情')} />
      </div>
    );
  }

  return (
    <div style={{ padding: '8px 12px' }}>
      <Space vertical align='start' style={{ width: '100%' }} spacing='medium'>
        {/* Channel & Retry Status */}
        <Card title={t('当前渠道 / 重试状态')} style={{ width: '100%' }}>
          <Space vertical align='start' style={{ width: '100%' }}>
            <Space align='center'>
              <Text strong>{t('当前渠道')}:</Text>
              {record.current_channel ? (
                <Tag color='blue'>
                  {record.current_channel.name || '-'} (ID: {record.current_channel.id || '-'}, {t('第{{num}}次', { num: record.current_channel.attempt || 1 })})
                </Tag>
              ) : (
                <Text type='tertiary'>{t('暂未选择渠道')}</Text>
              )}
            </Space>

            <Space align='center'>
              <Text strong>{t('当前响应状态')}:</Text>
              <Tag color={channelPhaseColors[record.current_phase] || 'grey'}>
                {phaseLabels[record.current_phase] || t('未知状态')}
              </Tag>
              {stopwatch.isActive && (
                <Text style={{
                  marginLeft: 12,
                  fontFamily: 'monospace',
                  color: '#666',
                  fontSize: '13px'
                }}>
                  {stopwatch.display}
                </Text>
              )}
            </Space>

            <div style={{ marginTop: '12px', width: '100%' }}>
              <Text strong style={{ display: 'block', marginBottom: '8px' }}>
                {t('渠道重试历史')}
              </Text>
              {(record.channel_attempts || []).length === 0 ? (
                <Text type='tertiary'>{t('暂无渠道重试记录')}</Text>
              ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                  {(record.channel_attempts || []).map((attempt, index) => {
                    const isActiveAttempt = index === activeAttemptIndex;
                    return (
                      <Card key={`${attempt.attempt}-${attempt.channel_id}-${attempt.started_at}`} size='small' bordered style={{ background: 'var(--semi-color-fill-0)' }}>
                        <Space vertical align='start' style={{ width: '100%' }}>
                          <Space align='center' style={{ justifyContent: 'space-between', width: '100%' }}>
                            <Space>
                              <Tag>{t('第{{num}}次', { num: attempt.attempt })}</Tag>
                              <Text>{attempt.channel_name || t('未知渠道')} (ID: {attempt.channel_id || '-'})</Text>
                            </Space>
                            <Space>
                              <Tag color={attemptStatusColors[attempt.status] || 'grey'}>
                                {attemptLabels[attempt.status] || attempt.status || t('未知状态')}
                              </Tag>
                              {isActive && isActiveAttempt && (
                                <Tooltip content={t('中断当前请求并尝试下一个渠道')}>
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
                            </Space>
                          </Space>
                          <Text type='tertiary' size='small'>
                            {t('开始')}: {attempt.started_at ? new Date(attempt.started_at).toLocaleTimeString() : '-'}
                            {attempt.ended_at ? ` | ${t('结束')}: ${new Date(attempt.ended_at).toLocaleTimeString()}` : ''}
                          </Text>
                          {(attempt.reason || attempt.error_code || attempt.http_status) && (
                            <Text size='small' style={{ color: 'var(--semi-color-text-2)' }}>
                              {t('原因')}: {attempt.reason || '-'}
                              {attempt.error_code ? ` | ${t('错误码')}: ${attempt.error_code}` : ''}
                              {attempt.http_status ? ` | HTTP ${attempt.http_status}` : ''}
                            </Text>
                          )}
                        </Space>
                      </Card>
                    );
                  })}
                </div>
              )}
              {interruptError && (
                <div style={{ marginTop: '8px', padding: '8px', background: 'var(--semi-color-danger-light-default)', borderRadius: '4px' }}>
                  <Text type='danger' size='small'>{t('中断失败')}: {interruptError}</Text>
                </div>
              )}
            </div>
          </Space>
        </Card>

        {/* Basic Info */}
        <Card title={t('请求信息')} style={{ width: '100%' }}>
          <Descriptions
            data={[
              { key: t('请求ID'), value: record.id },
              {
                key: t('状态'),
                value: (
                  <Tag color={statusColors[displayStatus] || statusColors[record.status] || 'grey'}>
                    {statusLabels[displayStatus] || statusLabels[record.status] || displayStatus || record.status || t('未知状态')}
                  </Tag>
                ),
              },
              {
                key: t('模型'),
                value: record.model ? (
                  renderModelTag(record.model, { shape: 'circle', size: 'small' })
                ) : (
                  <Text type='tertiary'>-</Text>
                ),
              },
              {
                key: t('是否流式'),
                value: record.is_stream ? (
                  <Tag color='blue'>{t('是')}</Tag>
                ) : (
                  <Tag>{t('否')}</Tag>
                ),
              },
              {
                key: t('开始时间'),
                value: new Date(record.start_time).toLocaleString(),
              },
              {
                key: t('耗时'),
                value: renderDurationTag(record.duration_ms, t),
              },
              { key: t('用户ID'), value: record.user_id || '-' },
              { key: t('令牌'), value: record.token_name || '-' },
              {
                key: t('渠道'),
                value: record.channel_name ? (
                  <Tag
                    color={stringToColor(record.channel_name || String(record.channel_id || ''))}
                    shape='circle'
                    size='small'
                  >
                    {record.channel_name}
                  </Tag>
                ) : (
                  <Text type='tertiary'>-</Text>
                ),
              },
            ]}
          />
        </Card>

        {/* Downstream Request */}
        <Card title={t('下游请求（客户端）')} style={{ width: '100%' }}>
          <Collapse>
            <Collapse.Panel header={t('请求头')} itemKey='downstream-headers'>
              <HeadersViewer headers={record.downstream?.headers} t={t} />
            </Collapse.Panel>
            <Collapse.Panel header={t('请求体')} itemKey='downstream-body'>
              <JsonViewer data={record.downstream?.body} t={t} />
              {record.downstream?.body_size > 0 && (
                <Text
                  type='tertiary'
                  size='small'
                  style={{ marginTop: '8px', display: 'block' }}
                >
                  {t('大小: {{size}} 字节', { size: record.downstream.body_size })}
                </Text>
              )}
            </Collapse.Panel>
          </Collapse>
          <div style={{ marginTop: '12px' }}>
            <Text type='tertiary'>
              {record.downstream?.method} {record.downstream?.path}
            </Text>
            <br />
            <Text type='tertiary'>{t('客户端IP: {{ip}}', { ip: record.downstream?.client_ip })}</Text>
          </div>
        </Card>

        {/* Response */}
        {record.response && (
          <Card title={t('响应')} style={{ width: '100%' }}>
            <Descriptions
              data={[
                {
                  key: t('状态码'),
                  value: (
                    <Tag
                      color={
                        record.response.status_code >= 400 ? 'red' : 'green'
                      }
                    >
                      {record.response.status_code}
                    </Tag>
                  ),
                },
                {
                  key: t('提示词Tokens'),
                  value: record.response.prompt_tokens || 0,
                },
                {
                  key: t('补全Tokens'),
                  value: record.response.completion_tokens || 0,
                },
              ]}
            />
            {record.response.error && (
              <div
                style={{
                  marginTop: '12px',
                  padding: '12px',
                  background: 'var(--semi-color-danger-light-default)',
                  borderRadius: '6px',
                }}
              >
                <Text type='danger' strong>
                  {t('错误: {{message}}', { message: record.response.error.message })}
                </Text>
              </div>
            )}
            <Collapse style={{ marginTop: '12px' }}>
              <Collapse.Panel header={t('响应头')} itemKey='response-headers'>
                <HeadersViewer headers={record.response?.headers} t={t} />
              </Collapse.Panel>
              <Collapse.Panel header={t('响应体')} itemKey='response-body'>
                <JsonViewer data={record.response?.body} t={t} />
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
  const { summaries, stats, connected, reconnect, channelUpdates } = useMonitorWs();
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
  const [selectedId, setSelectedId] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [filter, setFilter] = useState('all');
  const tableRef = useRef(null);
  // Track previous status to detect status changes
  const prevStatusRef = useRef(new Map());

  const statusLabels = getStatusLabels(t);

  // Fetch detail when selection changes
  useEffect(() => {
    if (selectedId) {
      fetchDetail(selectedId);
    }
  }, [selectedId, fetchDetail]);

  // Apply live channel updates streamed over WebSocket
  useEffect(() => {
    if (selectedId && channelUpdates[selectedId]) {
      applyLiveUpdate(selectedId, channelUpdates[selectedId]);
    }
  }, [selectedId, channelUpdates, applyLiveUpdate]);

  // Invalidate cache when a request's status changes (e.g., processing -> completed)
  // This ensures we fetch fresh data with response details
  useEffect(() => {
    summaries.forEach((summary) => {
      const displayStatus = deriveDisplayStatus(summary);
      const prevStatus = prevStatusRef.current.get(summary.id);
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
  }, [summaries, selectedId, invalidateCache, fetchDetail]);

  // Clear cache on reconnect
  useEffect(() => {
    if (!connected) {
      clearCache({ preserveSelection: true });
      prevStatusRef.current.clear();
    }
  }, [connected, clearCache]);

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

  const filteredSummaries = summaries.filter((r) => {
    const displayStatus = deriveDisplayStatus(r);
    if (filter === 'all') return true;
    if (filter === 'processing') return isActiveStatus(displayStatus);
    return displayStatus === filter;
  });

  // Sort by start_time descending (newest first)
  const sortedSummaries = [...filteredSummaries].sort(
    (a, b) => new Date(b.start_time) - new Date(a.start_time)
  );

  const columns = [
    {
      title: t('时间'),
      dataIndex: 'start_time',
      width: 120,
      render: (time) => new Date(time).toLocaleTimeString(),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (_, record) => {
        const displayStatus = deriveDisplayStatus(record);
        return (
          <Tag color={statusColors[displayStatus] || statusColors[record.status] || 'grey'}>
            {statusLabels[displayStatus] || statusLabels[record.status] || displayStatus || record.status}
          </Tag>
        );
      },
    },
    {
      title: t('模型'),
      dataIndex: 'model',
      width: 200,
      ellipsis: true,
      render: (_, record) =>
        renderModelTag(record.model || t('未知模型'), {
          shape: 'circle',
        }),
    },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      width: 180,
      ellipsis: true,
      render: (_, record) => (
        <Tag
          color={stringToColor(record.channel_name || String(record.channel_id || ''))}
          shape='circle'
        >
          {record.channel_name || t('未知渠道')}
        </Tag>
      ),
    },
    {
      title: t('耗时'),
      dataIndex: 'duration_ms',
      width: 100,
      render: (_, record) => <DurationCell record={record} t={t} />,
    },
  ];

  return (
    <div style={{ height: 'calc(100vh - 64px)', display: 'flex', flexDirection: 'column', padding: '8px', marginTop: '64px' }}>
      <Card style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
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
            <Text type='tertiary'>
              {t('活跃: {{active}} | 总计: {{total}}', { active: stats.active || 0, total: stats.total || 0 })}
            </Text>
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
                <Badge count={stats.active || 0} type='primary' />
              </Space>
            }
            itemKey='processing'
          />
          <TabPane tab={t('已完成')} itemKey='completed' />
          <TabPane tab={t('错误')} itemKey='error' />
        </Tabs>

        <div style={{ flex: 1, minHeight: 0 }}>
          <Table
            ref={tableRef}
            columns={columns}
            dataSource={sortedSummaries}
            rowKey='id'
            pagination={false}
            size='small'
            scroll={{ y: 'calc(100vh - 232px)' }}
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
                description={
                  connected
                    ? t('暂无请求')
                    : t('正在连接服务器...')
                }
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
        width={1000}
        bodyStyle={{ padding: 0 }}
        style={{ top: 36 }}
      >
        <div style={{ maxHeight: '70vh', overflow: 'auto', padding: '12px' }}>
          <RequestDetail
            record={selectedDetail}
            loading={detailLoading}
            error={detailError}
            t={t}
            statusLabels={statusLabels}
            onInterrupt={interruptRequest}
            interrupting={interrupting}
          />
        </div>
      </Modal>
    </div>
  );
};

export default Monitor;
