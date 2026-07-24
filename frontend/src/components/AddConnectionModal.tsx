import { useState } from 'react';
import type { PlatformKind, ConnRole } from '../types';
import { api, type CreateConnectionInput } from '../api';

interface Props {
  defaultRole: ConnRole;
  onClose: () => void;
  onCreated: () => void;
}

const KIND_OPTIONS: { value: PlatformKind; label: string; hint: string }[] = [
  { value: 'vmware', label: 'VMware vCenter / ESXi', hint: 'Source hypervisor (vCenter or standalone ESXi)' },
  { value: 'proxmox', label: 'Proxmox VE', hint: 'Target platform (PVE cluster or single node)' },
  { value: 'hyperv', label: 'Hyper-V', hint: 'Roadmap source (Phase 12B); inventory preview only' },
];

export function AddConnectionModal({ defaultRole, onClose, onCreated }: Props) {
  const [kind, setKind] = useState<PlatformKind>(defaultRole === 'source' ? 'vmware' : 'proxmox');
  const [role, setRole] = useState<ConnRole>(defaultRole);
  const [name, setName] = useState('');
  const [endpoint, setEndpoint] = useState('');
  const [secretRef, setSecretRef] = useState('');
  const [insecureTls, setInsecureTls] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState('');
  const [testMsg, setTestMsg] = useState('');

  const buildInput = (): CreateConnectionInput => ({
    name: name.trim(),
    kind,
    role,
    endpoint: endpoint.trim(),
    insecure_tls: insecureTls,
    secret_ref: secretRef.trim(),
  });

  const submit = async () => {
    if (!endpoint.trim()) {
      setError('Endpoint (host or URL) is required.');
      return;
    }
    setSubmitting(true);
    setError('');
    setTestMsg('');
    try {
      await api.createConnection(buildInput());
      onCreated();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const test = async () => {
    if (!endpoint.trim()) {
      setError('Enter an endpoint before testing.');
      return;
    }
    setTesting(true);
    setError('');
    setTestMsg('');
    try {
      // Create then test, then remove if the user hasn't saved yet. For mock
      // mode this always succeeds. In production a real probe runs here.
      const conn = await api.createConnection(buildInput());
      const res = await api.testConnection(conn.id);
      setTestMsg(`${res.status.toUpperCase()}: ${res.message}`);
      // Remove the temp connection so the user can adjust and save cleanly.
      await api.deleteConnection(conn.id);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header>
          Add {role === 'source' ? 'Source' : 'Target'} Connection
        </header>
        <div className="modal-body">
          <div className="field">
            <label>Platform</label>
            <select value={kind} onChange={(e) => setKind(e.target.value as PlatformKind)}>
              {KIND_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>
              {KIND_OPTIONS.find((o) => o.value === kind)?.hint}
            </div>
          </div>

          <div className="field">
            <label>Role</label>
            <select value={role} onChange={(e) => setRole(e.target.value as ConnRole)}>
              <option value="source">Source (VMs to migrate from)</option>
              <option value="target">Target (VMs migrate to)</option>
            </select>
          </div>

          <div className="field">
            <label>Connection Name</label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={kind === 'vmware' ? 'vCenter Production' : 'Proxmox Cluster A'}
            />
          </div>

          <div className="field">
            <label>Endpoint (host or URL)</label>
            <input
              value={endpoint}
              onChange={(e) => setEndpoint(e.target.value)}
              placeholder={kind === 'vmware' ? 'vcenter.example.local' : 'pve-01.example.local:8006'}
            />
          </div>

          <div className="field">
            <label>Secret Reference (vault path or token id)</label>
            <input
              value={secretRef}
              onChange={(e) => setSecretRef(e.target.value)}
              placeholder={kind === 'vmware' ? 'vmw/prod-admin' : 'pve/prod-token'}
            />
            <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>
              Secrets are referenced by name and never stored in the UI or logs.
            </div>
          </div>

          <div className="field">
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={insecureTls}
                onChange={(e) => setInsecureTls(e.target.checked)}
                style={{ width: 'auto' }}
              />
              Skip TLS certificate validation (lab only)
            </label>
            <div style={{ fontSize: 11, color: '#ffb4b0', marginTop: 4 }}>
              Insecure TLS is never recommended for production.
            </div>
          </div>

          {testMsg && <div style={{ color: 'var(--green)', fontSize: 13 }}>{testMsg}</div>}
          {error && <div className="error-text">{error}</div>}
        </div>
        <footer>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn" onClick={test} disabled={testing || submitting}>
            {testing ? <span className="spinner" /> : 'Test'}
          </button>
          <button className="btn primary" onClick={submit} disabled={submitting}>
            {submitting ? <span className="spinner" /> : 'Add Connection'}
          </button>
        </footer>
      </div>
    </div>
  );
}