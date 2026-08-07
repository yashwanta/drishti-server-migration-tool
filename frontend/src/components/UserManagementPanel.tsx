import { useCallback, useEffect, useState } from 'react';
import { api } from '../api';
import type { ManagedUser, Role } from '../types';
import { PasswordInput } from './PasswordInput';

const roles: Role[] = ['viewer', 'planner', 'operator', 'approver', 'platform_admin', 'auditor'];

export function UserManagementPanel({ currentUserID }: { currentUserID: string }) {
  const [users, setUsers] = useState<ManagedUser[]>([]);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [selectedRoles, setSelectedRoles] = useState<Role[]>(['viewer']);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    try {
      setUsers(await api.listUsers());
      setError('');
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    }
  }, []);

  useEffect(() => { void load(); }, [load]);

  const toggleRole = (role: Role) => {
    setSelectedRoles((current) => current.includes(role) ? current.filter((value) => value !== role) : [...current, role]);
  };

  const create = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.createUser({ username, password, roles: selectedRoles });
      setUsername('');
      setPassword('');
      setSelectedRoles(['viewer']);
      await load();
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  };

  const deactivate = async (user: ManagedUser) => {
    if (!window.confirm(`Deactivate ${user.username}? Their active sessions will be invalidated.`)) return;
    setBusy(true);
    setError('');
    try {
      await api.deactivateUser(user.id);
      await load();
    } catch (value) {
      setError(value instanceof Error ? value.message : String(value));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="user-management" aria-label="User management">
      <form className="user-create" onSubmit={(event) => void create(event)}>
        <strong>Create user</strong>
        <div className="field">
          <label htmlFor="new-username">Username</label>
          <input id="new-username" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="off" required pattern="[A-Za-z0-9._-]{3,64}" />
        </div>
        <PasswordInput label="Initial password" value={password} onChange={setPassword} autoComplete="new-password" required />
        <fieldset className="role-picker">
          <legend>Roles</legend>
          {roles.map((role) => <label key={role}><input type="checkbox" checked={selectedRoles.includes(role)} onChange={() => toggleRole(role)} /> {role}</label>)}
        </fieldset>
        <button className="btn primary" type="submit" disabled={busy || selectedRoles.length === 0}>Create user</button>
      </form>
      {error && <div className="error-text user-management-error">{error}</div>}
      <div className="user-list">
        {users.map((user) => (
          <div className="user-row" key={user.id}>
            <div><strong>{user.username}</strong><div className="meta">{user.roles.join(', ')}</div></div>
            <span className={`badge ${user.active ? 'green' : 'red'}`}>{user.active ? 'active' : 'disabled'}</span>
            <button className="btn" type="button" disabled={busy || !user.active || user.id === currentUserID} onClick={() => void deactivate(user)}>Deactivate</button>
          </div>
        ))}
        {users.length === 0 && <div className="empty">No users found.</div>}
      </div>
    </section>
  );
}
