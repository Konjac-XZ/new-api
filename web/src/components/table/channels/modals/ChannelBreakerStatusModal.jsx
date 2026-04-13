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

import React from 'react';
import { Button, Modal, Typography } from '@douyinfe/semi-ui';
import ChannelBreakerStatusCard from './ChannelBreakerStatusCard';

const { Text } = Typography;

const ChannelBreakerStatusModal = ({
    visible,
    onCancel,
    channel,
    loading,
    onReset,
    resetLoading,
    t,
}) => {
    const breakerState = channel?.breaker_state;
    const canReset = channel?.id && breakerState?.dynamic_enabled;

    return (
        <Modal
            title={
                <div>
                    <div>{t('动态熔断详情')}</div>
                    {channel?.name && (
                        <Text type='tertiary' size='small'>
                            {channel.name}
                        </Text>
                    )}
                </div>
            }
            visible={visible}
            onCancel={onCancel}
            bodyStyle={{ maxHeight: '75vh', overflowY: 'auto' }}
            footer={
                <div className='flex justify-end gap-2'>
                    <Button onClick={onCancel}>{t('关闭')}</Button>
                    <Button
                        type='warning'
                        theme='solid'
                        loading={resetLoading}
                        disabled={!canReset || resetLoading}
                        onClick={() => {
                            Modal.confirm({
                                title: t('确定重置当前通道熔断状态？'),
                                content: t(
                                    '这会清空该通道的压力、失败计数、冷却状态与历史失败率，并恢复 HP。',
                                ),
                                centered: true,
                                onOk: () => onReset?.(channel),
                            });
                        }}
                    >
                        {t('重置当前通道熔断状态')}
                    </Button>
                </div>
            }
            width={860}
        >
            {breakerState ? (
                <ChannelBreakerStatusCard
                    breakerState={breakerState}
                    traces={channel?.trace_page?.items || []}
                    traceTotal={channel?.trace_page?.total || 0}
                    traceLoading={loading}
                    t={t}
                    visible={visible}
                />
            ) : (
                <Text type='tertiary'>
                    {loading ? t('加载中...') : t('暂无熔断状态数据')}
                </Text>
            )}
        </Modal>
    );
};

export default ChannelBreakerStatusModal;
