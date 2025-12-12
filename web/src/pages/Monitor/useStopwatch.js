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
  }, [requestDetail]);

  return { display, isActive };
};
