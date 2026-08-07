import { useState } from 'react';
import type { PlatformKind, ConnRole } from '../types';
import { api, type CreateConnectionInput } from '../api';
import { PasswordInput } from './PasswordInput';

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
  const [credentialMode, setCredentialMode] = useState<'direct' | 'reference'>(defaultRole === 'source' ? 'direct' : 'reference');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
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
    secret_ref: credentialMode === 'reference' ? secretRef.trim() : undefined,
    username: credentialMode === 'direct' ? username.trim() : undefined,
    password: credentialMode === 'direct' ? password : undefined,
  });

  const validate = () => {
    if (!endpoint.trim()) return 'Endpoint (host or URL) is required.';
    if (credentialMode === 'direct' && (!username.trim() || !password)) return 'Username and password are required.';
    if (credentialMode === 'reference' && !secretRef.trim()) return 'Secret reference is required.';
    return '';
  };

  const submit = async () => {
    const validationError = validate();
    if (validationError) {
	  setError(validationError);
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
    const validationError = validate();
    if (validationError) {
	  setError(validationError);
      return;
    }
    setTesting(true);
    setError('');
    setTestMsg('');
    try {
      const res = await api.probeConnection(buildInput());
      setTestMsg(`${res.status.toUpperCase()}: ${res.message}`);
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
            <select value={kind} onChange={(e) => {
              const nextKind = e.target.value as PlatformKind;
              setKind(nextKind);
              setCredentialMode(nextKind === 'vmware' ? 'direct' : 'reference');
            }}>
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
            <label>Credentials</label>
            <select value={credentialMode} onChange={(e) => setCredentialMode(e.target.value as 'direct' | 'reference')}>
              {kind === 'vmware' && <option value="direct">Username &amp; password</option>}
              <option value="reference">Secret reference</option>
            </select>
          </div>

          {credentialMode === 'direct' ? (
            <>
              <div className="field">
                <label>Username</label>
                <input
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder={kind === 'vmware' ? 'root or administrator@vsphere.local' : 'root@pam'}
                  autoComplete="username"
                />
              </div>
              <div>
                <PasswordInput label="Password" value={password} onChange={setPassword} placeholder="Password" autoComplete="current-password" />
                <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>
                  Kept only in backend memory for this session; never returned, persisted, or logged.
                </div>
              </div>
            </>
          ) : (
            <div className="field">
              <label>Secret Reference (vault path or token id)</label>
              <input
                value={secretRef}
                onChange={(e) => setSecretRef(e.target.value)}
                placeholder={kind === 'vmware' ? 'vmw/prod-admin' : 'pve/prod-token'}
              />
              <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 4 }}>
                Requires a configured secret provider.
              </div>
            </div>
          )}

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
