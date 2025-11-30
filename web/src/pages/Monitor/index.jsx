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
} from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { WrapText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import useMonitorWs from './useMonitorWs';
import useRequestDetail from './useRequestDetail';

const { Title, Text } = Typography;

const statusColors = {
  pending: 'grey',
  processing: 'blue',
  completed: 'green',
  error: 'red',
};

const getStatusLabels = (t) => ({
  pending: t('等待中'),
  processing: t('处理中'),
  completed: t('已完成'),
  error: t('错误'),
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

const RequestDetail = ({ record, loading, error, t, statusLabels }) => {
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
      <Space vertical align='start' style={{ width: '100%' }} spacing='small'>
        {/* Basic Info */}
        <Card title={t('请求信息')} style={{ width: '100%' }}>
          <Descriptions
            data={[
              { key: t('请求ID'), value: record.id },
              {
                key: t('状态'),
                value: (
                  <Tag color={statusColors[record.status]}>
                    {statusLabels[record.status]}
                  </Tag>
                ),
              },
              { key: t('模型'), value: record.model || '-' },
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
                value: record.duration_ms ? `${record.duration_ms}ms` : '-',
              },
              { key: t('用户ID'), value: record.user_id || '-' },
              { key: t('令牌'), value: record.token_name || '-' },
              { key: t('渠道'), value: record.channel_name || '-' },
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
              <Collapse.Panel header={t('请求头')} itemKey='response-headers'>
                <HeadersViewer headers={record.response?.headers} t={t} />
              </Collapse.Panel>
              <Collapse.Panel header={t('请求体')} itemKey='response-body'>
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
  const { summaries, stats, connected, reconnect } = useMonitorWs();
  const {
    selectedDetail,
    loading: detailLoading,
    error: detailError,
    fetchDetail,
    invalidateCache,
    clearCache,
  } = useRequestDetail();
  const [selectedId, setSelectedId] = useState(null);
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

  // Invalidate cache when a request's status changes (e.g., processing -> completed)
  // This ensures we fetch fresh data with response details
  useEffect(() => {
    summaries.forEach((summary) => {
      const prevStatus = prevStatusRef.current.get(summary.id);
      if (prevStatus && prevStatus !== summary.status) {
        // Status changed, invalidate cache to get fresh data
        invalidateCache(summary.id);

        // If this is the currently selected item, refetch
        if (selectedId === summary.id) {
          fetchDetail(summary.id);
        }
      }
      prevStatusRef.current.set(summary.id, summary.status);
    });
  }, [summaries, selectedId, invalidateCache, fetchDetail]);

  // Clear cache on reconnect
  useEffect(() => {
    if (!connected) {
      clearCache();
      prevStatusRef.current.clear();
    }
  }, [connected, clearCache]);

  const handleRowClick = useCallback((record) => {
    setSelectedId(record.id);
  }, []);

  const filteredSummaries = summaries.filter((r) => {
    if (filter === 'all') return true;
    return r.status === filter;
  });

  // Sort by start_time descending (newest first)
  const sortedSummaries = [...filteredSummaries].sort(
    (a, b) => new Date(b.start_time) - new Date(a.start_time)
  );

  const columns = [
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (status) => (
        <Tag color={statusColors[status]}>{statusLabels[status]}</Tag>
      ),
    },
    {
      title: t('模型'),
      dataIndex: 'model',
      width: 150,
      ellipsis: true,
    },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      width: 120,
      ellipsis: true,
    },
    {
      title: t('耗时'),
      dataIndex: 'duration_ms',
      width: 100,
      render: (duration) => (duration ? `${duration}ms` : '-'),
    },
    {
      title: t('时间'),
      dataIndex: 'start_time',
      width: 180,
      render: (time) => new Date(time).toLocaleTimeString(),
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

        <div style={{ display: 'flex', gap: '16px', flex: 1, minHeight: 0 }}>
          {/* Request List */}
          <div style={{ flex: '0 0 50%', maxWidth: '50%', display: 'flex', flexDirection: 'column' }}>
            <Table
              ref={tableRef}
              columns={columns}
              dataSource={sortedSummaries}
              rowKey='id'
              pagination={false}
              size='small'
              scroll={{ y: 'calc(100vh - 284px)' }}
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

          {/* Request Detail */}
          <div
            style={{
              flex: '1',
              borderLeft: '1px solid var(--semi-color-border)',
              overflow: 'auto',
              height: 'calc(100vh - 284px)',
            }}
          >
            <RequestDetail
              record={selectedDetail}
              loading={detailLoading}
              error={detailError}
              t={t}
              statusLabels={statusLabels}
            />
          </div>
        </div>
      </Card>
    </div>
  );
};

export default Monitor;
