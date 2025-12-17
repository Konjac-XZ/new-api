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
