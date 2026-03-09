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
import { Modal, Typography } from '@douyinfe/semi-ui';
import ChannelBreakerStatusCard from './ChannelBreakerStatusCard';

const { Text } = Typography;

const ChannelBreakerStatusModal = ({
    visible,
    onCancel,
    channel,
    t,
}) => {
    const breakerState = channel?.breaker_state;

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
            footer={null}
            width={480}
        >
            {breakerState ? (
                <ChannelBreakerStatusCard
                    breakerState={breakerState}
                    t={t}
                    visible={visible}
                />
            ) : (
                <Text type='tertiary'>{t('暂无熔断状态数据')}</Text>
            )}
        </Modal>
    );
};

export default ChannelBreakerStatusModal;
