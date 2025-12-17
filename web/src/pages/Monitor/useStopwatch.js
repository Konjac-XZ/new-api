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
      const startedAt = new Date(currentAttempt.started_at).getTime();

      if (status === 'waiting_upstream') {
        const elapsed = (now - startedAt) / 1000;
        setDisplay(`${t('等待')}: ${elapsed.toFixed(1)}s`);
      } else if (status === 'streaming') {
        const streamingStartedAt = currentAttempt.streaming_started_at
          ? new Date(currentAttempt.streaming_started_at).getTime()
          : null;

        if (streamingStartedAt) {
          const waitingTime = (streamingStartedAt - startedAt) / 1000;
          const streamingTime = (now - streamingStartedAt) / 1000;
          setDisplay(`${t('等待')}: ${waitingTime.toFixed(1)}s | ${t('流式返回')}: ${streamingTime.toFixed(1)}s`);
        } else {
          // Fallback if streaming_started_at is missing
          const totalTime = (now - startedAt) / 1000;
          setDisplay(`${t('流式返回')}: ${totalTime.toFixed(1)}s`);
        }
      }
    };

    updateDisplay();
    const interval = setInterval(updateDisplay, 100);

    return () => clearInterval(interval);
  }, [requestDetail, t]);

  return { display, isActive };
};
