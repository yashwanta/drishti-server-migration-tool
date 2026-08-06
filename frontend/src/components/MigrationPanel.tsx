import { useState } from 'react';
import { api } from '../api';
import type { Plan } from '../types';

interface Props {
  plans: Plan[];
  onRefresh: () => void;
}

export function MigrationPanel({ plans, onRefresh }: Props) {
  const [busy, setBusy] = useState<string>('');
  const [msg, setMsg] = useState('');

  const act = async (label: string, planId: string, fn: () => Promise<unknown>) => {
    setBusy(label + ':' + planId);
    setMsg('');
    try {
      const res = await fn();
      setMsg(label + ' OK: ' + JSON.stringify(res).substring(0, 200));
      onRefresh();
    } catch (e) {
      setMsg(label + ' FAILED: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy('');
    }
  };

  const jobForPlan = (planId: string) => 'job-' + planId;

  return (
    <div>
      {plans.length === 0 && <div className="empty">No plans yet. Drag a VM onto a Proxmox node.</div>}
      {plans.map((p) => (
        <div key={p.id} style={{ padding: '8px 0', borderBottom: '1px solid var(--border)' }}>
          <div style={{ fontWeight: 600, fontSize: 13 }}>{p.name}</div>
          <div className="meta" style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 6 }}>
            {p.source_vm_id} {'->'} {p.target_node_id} -{' '}
            <span className={('badge ' + (p.status === 'approved' ? 'green' : p.status === 'draft' ? 'amber' : ''))}>{p.status}</span>
            {' '}<span className="badge">{p.strategy === 'pve-live' ? 'live' : 'cold'}</span>
          </div>
          <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11 }}
              disabled={busy.length > 0}
              onClick={() => act('Preflight', p.id, () => api.runPreflight(p.id))}
            >
              {busy === 'Preflight:' + p.id ? <span className="spinner" /> : 'Preflight'}
            </button>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11 }}
              disabled={busy.length > 0 || p.status !== 'preflight' || !p.preflight_passed}
              onClick={() => {
                if (!confirm('Approve this migration plan? Approval alone does not start migration.')) return;
                const approvePowerOff = confirm(
                  'Separately authorize DRISHTI to power off this exact source VM if it is running?\n\nChoose Cancel if the VM is already off or you will stop it manually. DRISHTI will never power it back on, delete it, or unregister it.',
                );
                act('Approve', p.id, () => api.approvePlan(p.id, approvePowerOff));
              }}
            >
              Approve
            </button>
            <button
              className="btn primary"
              style={{ padding: '3px 8px', fontSize: 11 }}
              disabled={busy.length > 0 || p.status !== 'approved'}
              onClick={() => act('Execute', p.id, () => api.executeMigration(p.id))}
            >
              {busy === 'Execute:' + p.id ? <span className="spinner" /> : 'Execute'}
            </button>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11 }}
              disabled={busy.length > 0}
              onClick={() => act('Validate', p.id, () => api.validateJob(jobForPlan(p.id)))}
            >
              Validate
            </button>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11, borderColor: 'var(--green)', color: 'var(--green)' }}
              disabled={busy.length > 0}
              onClick={() => act('Cutover', p.id, () => api.cutoverJob(jobForPlan(p.id)))}
            >
              Cutover
            </button>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11, borderColor: 'var(--red)', color: 'var(--red)' }}
              disabled={busy.length > 0}
              onClick={() => { if (confirm('Prepare rollback? The target will be isolated and stopped. DRISHTI will not power on the VMware source; an authorized operator must do that manually.')) act('Rollback', p.id, () => api.rollbackJob(jobForPlan(p.id))) }}
            >
              Rollback
            </button>
            <button
              className="btn"
              style={{ padding: '3px 8px', fontSize: 11 }}
              disabled={busy.length > 0}
              onClick={() => act('Report', p.id, () => api.getReport(jobForPlan(p.id)))}
            >
              Report
            </button>
          </div>
        </div>
      ))}
      {msg && <div style={{ marginTop: 8, fontSize: 11, color: 'var(--muted)', whiteSpace: 'pre-wrap', maxHeight: 200, overflow: 'auto' }}>{msg}</div>}
    </div>
  );
}
