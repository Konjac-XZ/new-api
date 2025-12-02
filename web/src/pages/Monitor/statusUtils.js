const ACTIVE_STATUSES = ['processing', 'waiting_upstream', 'streaming'];
const TERMINAL_STATUSES = ['completed', 'error'];

export const deriveDisplayStatus = (record) => {
  if (!record) return '';

  const status = record.status;
  const currentPhase = record.current_phase;

  if (status === 'completed' || status === 'error') {
    return status;
  }

  if (status === 'waiting_upstream' || status === 'streaming') {
    return status;
  }

  if (status === 'processing' || status === 'pending') {
    if (currentPhase === 'streaming' || currentPhase === 'waiting_upstream') {
      return currentPhase;
    }
  }

  if (currentPhase === 'streaming' || currentPhase === 'waiting_upstream') {
    return currentPhase;
  }

  return status || '';
};

export const isActiveStatus = (status) => ACTIVE_STATUSES.includes(status);

export const isTerminalStatus = (status) => TERMINAL_STATUSES.includes(status);
