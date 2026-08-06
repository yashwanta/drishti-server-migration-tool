import type { ApiError, AuditEvent, Connection, InventoryRoot, Job, Plan, PlatformKind, ConnRole, PreflightCheck, Session } from './types';

const BASE = '/api/v1';
let csrfToken = '';

export class ApiRequestError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message);
    this.name = 'ApiRequestError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...((init?.method && !['GET', 'HEAD'].includes(init.method)) && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
      ...(init?.headers ?? {}),
    },
    ...init,
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = (await res.json()) as ApiError;
      detail = body.detail ? `${body.error}: ${body.detail}` : body.error;
    } catch {}
    throw new ApiRequestError(res.status, `API ${res.status}: ${detail}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

async function health(): Promise<{ status: string; mode: string }> {
  const res = await fetch('/healthz');
  if (!res.ok) throw new Error(`Health check failed: HTTP ${res.status}`);
  return (await res.json()) as { status: string; mode: string };
}

export interface CreateConnectionInput {
  name: string;
  kind: PlatformKind;
  role: ConnRole;
  endpoint: string;
  insecure_tls: boolean;
  secret_ref?: string;
  username?: string;
  password?: string;
}
export interface CreatePlanInput { name: string; source_vm_id: string; source_connection_id: string; target_node_id: string; target_connection_id: string; target_vm_name: string; cpu: number; memory_mb: number; firmware: 'bios' | 'uefi'; disk_format: 'raw' | 'qcow2'; strategy: 'cold' | 'pve-live'; storage_maps: { source_disk_id: string; target_storage_id: string; target_format: 'raw' | 'qcow2' }[]; network_maps: { source_nic_id: string; target_bridge: string; vlan_id?: number }[]; }

export const api = {
  health,
  session: () => request<Session>('/auth/session').then((session) => {
    csrfToken = session.csrf_token;
    return session;
  }),
  login: (username: string, password: string) => request<Session>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }).then((session) => {
    csrfToken = session.csrf_token;
    return session;
  }),
  logout: () => request<void>('/auth/logout', { method: 'POST' }).finally(() => { csrfToken = ''; }),
  listConnections: () => request<{ connections: Connection[] }>('/connections').then((r) => r.connections),
  getConnection: (id: string) => request<Connection>(`/connections/${id}`),
  createConnection: (input: CreateConnectionInput) => request<Connection>('/connections', { method: 'POST', body: JSON.stringify(input) }),
  probeConnection: (input: CreateConnectionInput) => request<{ status: string; message: string }>('/connections/probe', { method: 'POST', body: JSON.stringify(input) }),
  deleteConnection: (id: string) => request<void>(`/connections/${id}`, { method: 'DELETE' }),
  testConnection: (id: string) => request<{ connection_id: string; status: string; message: string }>(`/connections/${id}/test`, { method: 'POST' }),
  getInventory: (id: string) => request<InventoryRoot>(`/connections/${id}/inventory`),
  getRuntime: () => request<{ mode: 'mock' | 'lab' | 'live' | 'production' }>('/runtime'),
  listPlans: () => request<{ plans: Plan[] }>('/plans').then((r) => r.plans),
  createPlan: (input: CreatePlanInput) => request<Plan>('/plans', { method: 'POST', body: JSON.stringify(input) }),
  getPlan: (id: string) => request<Plan>(`/plans/${id}`),
  deletePlan: (id: string) => request<void>(`/plans/${id}`, { method: 'DELETE' }),
  runPreflight: (planId: string) => request<{ plan_id: string; pass: boolean; checks: PreflightCheck[]; blocked?: string[] }>(`/plans/${planId}/preflight`, { method: 'POST' }),
  approvePlan: (planId: string, approveSourcePowerOff: boolean) => request<Plan>(`/plans/${planId}/approve`, { method: 'POST', body: JSON.stringify({ approve_source_power_off: approveSourcePowerOff }) }),
  executeMigration: (planId: string) => request<unknown>(`/plans/${planId}/execute`, { method: 'POST' }),
  listJobs: () => request<{ jobs: Job[] }>('/jobs').then((r) => r.jobs),
  getJob: (id: string) => request<Job>(`/jobs/${id}`),
  listAudit: () => request<{ events: AuditEvent[] }>('/audit').then((r) => r.events),
  validateJob: (jobId: string) => request<{ passed: boolean; checks: { name: string; status: string; detail: string }[] }>(`/jobs/${jobId}/validate`, { method: 'POST' }),
  cutoverJob: (jobId: string) => request<{ success: boolean; steps: string[]; warning?: string; retention_deadline?: string }>(`/jobs/${jobId}/cutover`, { method: 'POST' }),
  rollbackJob: (jobId: string) => request<{ success: boolean; steps: string[]; warning?: string }>(`/jobs/${jobId}/rollback`, { method: 'POST' }),
  getReport: (jobId: string) => request<unknown>(`/jobs/${jobId}/report`),
};
