import { useState } from 'react';
import type { FormEvent } from 'react';
import { api } from '../api';
import type { Session } from '../types';
import { PasswordInput } from './PasswordInput';

export function LoginPanel({ onLogin }: { onLogin: (session: Session) => void }) {
  const [changingPassword, setChangingPassword] = useState(false);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const resetMessages = () => { setError(''); setMessage(''); };

  const signIn = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    resetMessages();
    try {
      onLogin(await api.login(username, password));
    } catch {
      setError('Invalid username or password.');
    } finally {
      setSubmitting(false);
    }
  };

  const changePassword = async (event: FormEvent) => {
    event.preventDefault();
    resetMessages();
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match.');
      return;
    }
    setSubmitting(true);
    let temporarySession = false;
    try {
      await api.login(username, password);
      temporarySession = true;
      await api.changePassword(password, newPassword);
      temporarySession = false;
      setChangingPassword(false);
      setPassword('');
      setNewPassword('');
      setConfirmPassword('');
      setMessage('Password changed. Sign in with your new password.');
    } catch {
      setError('Unable to change password. Verify your credentials and password requirements.');
    } finally {
      if (temporarySession) await api.logout().catch(() => undefined);
      setSubmitting(false);
    }
  };

  return (
    <div className="app">
      <div className="panel login-panel">
        <header>{changingPassword ? 'Change your password' : 'Sign in to DRISHTI HyperShift'}</header>
        <form className="body" onSubmit={(event) => void (changingPassword ? changePassword(event) : signIn(event))}>
          <div className="field">
            <label htmlFor="login-username">Username</label>
            <input id="login-username" value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" required />
          </div>
          <PasswordInput label="Current password" value={password} onChange={setPassword} autoComplete="current-password" required />
          {changingPassword && (
            <>
              <PasswordInput label="New password" value={newPassword} onChange={setNewPassword} autoComplete="new-password" required />
              <PasswordInput label="Confirm new password" value={confirmPassword} onChange={setConfirmPassword} autoComplete="new-password" required />
              <div className="meta password-hint">Use 12–72 characters.</div>
            </>
          )}
          {error && <div className="error-text">{error}</div>}
          {message && <div className="success-text">{message}</div>}
          <div className="login-actions">
            <button className="btn primary" type="submit" disabled={submitting}>
              {submitting ? 'Please wait...' : changingPassword ? 'Change password' : 'Sign in'}
            </button>
            <button className="link-button" type="button" disabled={submitting} onClick={() => { setChangingPassword((value) => !value); resetMessages(); }}>
              {changingPassword ? 'Back to sign in' : 'Change password'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
