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
import { Button, Modal, Select, Typography } from '@douyinfe/semi-ui';
import {
  IconArrowDown,
  IconArrowUp,
  IconDelete,
  IconPlus,
} from '@douyinfe/semi-icons';

const DEFAULT_SORT_RULES = [{ field: 'priority', order: 'desc' }];
const EXAMPLE_SORT_RULES = [
  { field: 'priority', order: 'desc' },
  { field: 'weight', order: 'desc' },
  { field: 'name', order: 'asc' },
  { field: 'id', order: 'desc' },
];

const SORT_FIELD_OPTIONS = [
  { value: 'priority', label: '优先级' },
  { value: 'weight', label: '权重' },
  { value: 'name', label: '名称' },
  { value: 'id', label: 'ID' },
  { value: 'balance', label: '余额' },
  { value: 'response_time', label: '响应时间' },
  { value: 'test_time', label: '测试时间' },
];

const SORT_ORDER_OPTIONS = [
  { value: 'desc', label: '降序' },
  { value: 'asc', label: '升序' },
];

const normalizeRules = (rules) => {
  const seen = new Set();
  const normalized = [];
  (Array.isArray(rules) ? rules : DEFAULT_SORT_RULES).forEach((rule) => {
    const field = rule?.field;
    if (!field || seen.has(field)) {
      return;
    }
    seen.add(field);
    normalized.push({
      field,
      order: rule.order === 'asc' ? 'asc' : 'desc',
    });
  });
  return normalized.length > 0 ? normalized : [...DEFAULT_SORT_RULES];
};

const ChannelSortModal = ({
  visible,
  onCancel,
  channelSortRules,
  applyChannelSortRules,
  t,
}) => {
  const [draftRules, setDraftRules] = useState(() =>
    normalizeRules(channelSortRules),
  );

  const fieldOptions = useMemo(
    () =>
      SORT_FIELD_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const orderOptions = useMemo(
    () =>
      SORT_ORDER_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  useEffect(() => {
    if (visible) {
      setDraftRules(normalizeRules(channelSortRules));
    }
  }, [visible, channelSortRules]);

  const updateRule = (index, patch) => {
    setDraftRules((rules) =>
      normalizeRules(
        rules.map((rule, ruleIndex) =>
          ruleIndex === index ? { ...rule, ...patch } : rule,
        ),
      ),
    );
  };

  const moveRule = (index, offset) => {
    setDraftRules((rules) => {
      const nextIndex = index + offset;
      if (nextIndex < 0 || nextIndex >= rules.length) {
        return rules;
      }
      const nextRules = [...rules];
      [nextRules[index], nextRules[nextIndex]] = [
        nextRules[nextIndex],
        nextRules[index],
      ];
      return nextRules;
    });
  };

  const addRule = () => {
    setDraftRules((rules) => {
      const usedFields = new Set(rules.map((rule) => rule.field));
      const nextField = SORT_FIELD_OPTIONS.find(
        (option) => !usedFields.has(option.value),
      )?.value;
      if (!nextField) {
        return rules;
      }
      return [...rules, { field: nextField, order: 'desc' }];
    });
  };

  const deleteRule = (index) => {
    setDraftRules((rules) =>
      normalizeRules(rules.filter((_, ruleIndex) => ruleIndex !== index)),
    );
  };

  const applyRules = () => {
    applyChannelSortRules(draftRules);
    onCancel();
  };

  return (
    <Modal
      title={t('渠道排序规则')}
      visible={visible}
      onCancel={onCancel}
      width={720}
      footer={
        <div className='flex flex-wrap justify-between gap-2'>
          <div className='flex flex-wrap gap-2'>
            <Button
              size='small'
              type='tertiary'
              onClick={() => setDraftRules([...DEFAULT_SORT_RULES])}
            >
              {t('恢复默认')}
            </Button>
            <Button
              size='small'
              type='tertiary'
              onClick={() => setDraftRules([...EXAMPLE_SORT_RULES])}
            >
              {t('使用示例规则')}
            </Button>
          </div>
          <div className='flex gap-2'>
            <Button onClick={onCancel}>{t('取消')}</Button>
            <Button type='primary' onClick={applyRules}>
              {t('应用')}
            </Button>
          </div>
        </div>
      }
    >
      <div className='flex flex-col gap-3'>
        <Typography.Text type='secondary'>
          {t(
            '按从上到下的顺序依次比较：当前一项相同时，继续使用下一项排序。',
          )}
        </Typography.Text>

        {draftRules.map((rule, index) => {
          const selectedFields = new Set(
            draftRules
              .filter((_, ruleIndex) => ruleIndex !== index)
              .map((item) => item.field),
          );
          const availableFields = fieldOptions.map((option) => ({
            ...option,
            disabled: selectedFields.has(option.value),
          }));

          return (
            <div
              key={`${rule.field}-${index}`}
              className='flex flex-col md:flex-row md:items-center gap-2 rounded p-3'
              style={{ border: '1px solid var(--semi-color-border)' }}
            >
              <Typography.Text strong className='w-16'>
                {t('第 {{index}} 级', { index: index + 1 })}
              </Typography.Text>
              <Select
                size='small'
                value={rule.field}
                optionList={availableFields}
                onChange={(field) => updateRule(index, { field })}
                className='w-full md:w-48'
              />
              <Select
                size='small'
                value={rule.order}
                optionList={orderOptions}
                onChange={(order) => updateRule(index, { order })}
                className='w-full md:w-28'
              />
              <div className='flex gap-1 md:ml-auto'>
                <Button
                  size='small'
                  theme='borderless'
                  icon={<IconArrowUp />}
                  disabled={index === 0}
                  onClick={() => moveRule(index, -1)}
                  aria-label={t('上移')}
                />
                <Button
                  size='small'
                  theme='borderless'
                  icon={<IconArrowDown />}
                  disabled={index === draftRules.length - 1}
                  onClick={() => moveRule(index, 1)}
                  aria-label={t('下移')}
                />
                <Button
                  size='small'
                  type='danger'
                  theme='borderless'
                  icon={<IconDelete />}
                  disabled={draftRules.length === 1}
                  onClick={() => deleteRule(index)}
                  aria-label={t('删除')}
                />
              </div>
            </div>
          );
        })}

        <Button
          size='small'
          icon={<IconPlus />}
          onClick={addRule}
          disabled={draftRules.length >= SORT_FIELD_OPTIONS.length}
        >
          {t('添加排序级别')}
        </Button>
      </div>
    </Modal>
  );
};

export default ChannelSortModal;
