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

import { useState, useEffect } from 'react';
import { DURATION_UPDATE_INTERVAL_MS, MS_TO_SECONDS } from './constants';

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

const formatLiveSeconds = (seconds) => {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return '0.0';
  }

  return (Math.floor(seconds * 10) / 10).toFixed(1);
};

export const useStopwatch = (requestDetail, t) => {
  const [display, setDisplay] = useState('');
  const [isActive, setIsActive] = useState(false);

  useEffect(() => {
    if (!requestDetail) return;

    // Find current active attempt
    const attempts = requestDetail.channel_attempts || [];
    const currentAttempt = attempts[attempts.length - 1];

    if (!currentAttempt || currentAttempt.ended_at) {
      setIsActive(false);
      return;
    }

    const status = currentAttempt.status;
    if (status !== 'waiting_upstream' && status !== 'streaming') {
      setIsActive(false);
      return;
    }

    setIsActive(true);

    const updateDisplay = () => {
      const now = Date.now();
      const startedAt = getTimestampMs(
        currentAttempt.started_at_ms,
        currentAttempt.started_at,
      );

      if (!startedAt) {
        setDisplay('');
        return;
      }

      if (status === 'waiting_upstream') {
        const elapsed = (now - startedAt) / MS_TO_SECONDS;
        setDisplay(`${t('等待')}: ${formatLiveSeconds(elapsed)}s`);
      } else if (status === 'streaming') {
        const streamingStartedAt = getTimestampMs(
          currentAttempt.streaming_started_at_ms,
          currentAttempt.streaming_started_at,
        );

        if (streamingStartedAt) {
          const waitingTime = (streamingStartedAt - startedAt) / MS_TO_SECONDS;
          const streamingTime = (now - streamingStartedAt) / MS_TO_SECONDS;
          setDisplay(`${t('等待')}: ${formatLiveSeconds(waitingTime)}s | ${t('流式返回')}: ${formatLiveSeconds(streamingTime)}s`);
        } else {
          // Fallback if streaming_started_at is missing
          const totalTime = (now - startedAt) / MS_TO_SECONDS;
          setDisplay(`${t('流式返回')}: ${formatLiveSeconds(totalTime)}s`);
        }
      }
    };

    updateDisplay();
    const interval = setInterval(updateDisplay, DURATION_UPDATE_INTERVAL_MS);

    return () => clearInterval(interval);
  }, [requestDetail, t]);

  return { display, isActive };
};
