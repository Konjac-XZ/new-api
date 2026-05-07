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

import React, { useEffect, useMemo, useState } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getChannelsColumns } from './ChannelsColumnDefs';

const ChannelsTable = (channelsData) => {
  const {
    channels,
    displayChannels,
    loading,
    searching,
    activePage,
    pageSize,
    channelCount,
    enableBatchDelete,
    compactMode,
    isDashboardMode,
    visibleColumns,
    setSelectedChannels,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    t,
    COLUMN_KEYS,
    // Column functions and data
    updateChannelBalance,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    testChannelStream,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    checkOllamaVersion,
    // Multi-key management
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
    // Breaker status modal
    setShowBreakerStatusModal,
    setCurrentBreakerStatusChannel,
  } = channelsData;

  const [currentUnixSeconds, setCurrentUnixSeconds] = useState(
    Math.floor(Date.now() / 1000),
  );

  useEffect(() => {
    const tick = () => {
      setCurrentUnixSeconds(Math.floor(Date.now() / 1000));
    };
    tick();
    const timer = window.setInterval(tick, 1000);
    return () => {
      window.clearInterval(timer);
    };
  }, []);

  // Get all columns
  const allColumns = useMemo(() => {
    return getChannelsColumns({
      t,
      COLUMN_KEYS,
      updateChannelBalance,
      manageChannel,
      manageTag,
      submitTagEdit,
      testChannel,
      testChannelStream,
      setCurrentTestChannel,
      setShowModelTestModal,
      setEditingChannel,
      setShowEdit,
      setShowEditTag,
      setEditingTag,
      copySelectedChannel,
      refresh,
      activePage,
      channels,
      checkOllamaVersion,
      setShowMultiKeyManageModal,
      setCurrentMultiKeyChannel,
      openUpstreamUpdateModal,
      detectChannelUpstreamUpdates,
      setShowBreakerStatusModal,
      setCurrentBreakerStatusChannel,
      isDashboardMode,
      currentUnixSeconds,
    });
  }, [
    t,
    COLUMN_KEYS,
    updateChannelBalance,
    manageChannel,
    manageTag,
    submitTagEdit,
    testChannel,
    testChannelStream,
    setCurrentTestChannel,
    setShowModelTestModal,
    setEditingChannel,
    setShowEdit,
    setShowEditTag,
    setEditingTag,
    copySelectedChannel,
    refresh,
    activePage,
    channels,
    checkOllamaVersion,
    setShowMultiKeyManageModal,
    setCurrentMultiKeyChannel,
    openUpstreamUpdateModal,
    detectChannelUpstreamUpdates,
    setShowBreakerStatusModal,
    setCurrentBreakerStatusChannel,
    isDashboardMode,
    currentUnixSeconds,
  ]);

  // Filter columns based on visibility settings
  const getVisibleColumns = () => {
    return allColumns.filter(
      (column) =>
        visibleColumns[column.key] &&
        (!isDashboardMode || column.key !== COLUMN_KEYS.OPERATE),
    );
  };

  const visibleColumnsList = useMemo(() => {
    return getVisibleColumns();
  }, [visibleColumns, allColumns, isDashboardMode, COLUMN_KEYS.OPERATE]);

  const tableColumns = useMemo(() => {
    return compactMode
      ? visibleColumnsList.map(({ fixed, ...rest }) => rest)
      : visibleColumnsList;
  }, [compactMode, visibleColumnsList]);

  return (
    <CardTable
      columns={tableColumns}
      dataSource={displayChannels}
      scroll={compactMode ? undefined : { x: 'max-content' }}
      pagination={{
        currentPage: activePage,
        pageSize: pageSize,
        total: channelCount,
        pageSizeOpts: [10, 20, 50, 100],
        showSizeChanger: true,
        onPageSizeChange: handlePageSizeChange,
        onPageChange: handlePageChange,
      }}
      hidePagination={true}
      expandAllRows={false}
      onRow={handleRow}
      rowSelection={
        enableBatchDelete && !isDashboardMode
          ? {
            onChange: (selectedRowKeys, selectedRows) => {
              setSelectedChannels(selectedRows);
            },
          }
          : null
      }
      empty={
        <Empty
          image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
          darkModeImage={
            <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
          }
          description={t('搜索无结果')}
          style={{ padding: 30 }}
        />
      }
      className='rounded-xl overflow-hidden'
      size='middle'
      loading={loading || searching}
    />
  );
};

export default ChannelsTable;
