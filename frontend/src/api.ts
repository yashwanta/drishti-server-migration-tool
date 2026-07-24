import type { ApiError, Connection, InventoryRoot, Plan } from './types';

const BASE = '/api/v1';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
    ...init,
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = (await res.json()) as ApiError;
      detail = body.detail ? `${body.error}: ${body.detail}` : body.error;
    } catch {
      // keep default status text
    }
    throw new Error(`API ${res.status}: ${detail}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  listConnections: () =>
    request<{ connections: Connection[] }>('/connections').then((r) => r.connections),

  getConnection: (id: string) => request<Connection>(`/connections/${id}`),

  getInventory: (id: string) => request<InventoryRoot>(`/connections/${id}/inventory`),

  listPlans: () => request<{ plans: Plan[] }>('/plans').then((r) => r.plans),

  createPlan: (input: CreatePlanInput) =>
    request<Plan>('/plans', { method: 'POST', body: JSON.stringify(input) }),

  getPlan: (id: string) => request<Plan>(`/plans/${id}`),

  deletePlan: (id: string) =>
    request<void>(`/plans/${id}`, { method: 'DELETE' }),
};

export interface CreatePlanInput {
  name: string;
  source_vm_id: string;
  source_connection_id: string;
  target_node_id: string;
  target_connection_id: string;
  target_vm_name: string;
  cpu: number;
  memory_mb: number;
  firmware: 'bios' | 'uefi';
}