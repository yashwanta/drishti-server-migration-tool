import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { api, ApiRequestError } from '../api';
import type { AuditEvent, Job, JobState, Plan } from '../types';

const POLL_INTERVAL_MS = 5_000;
const MAX_POLL_FAILURES = 5;

interface Props {
  plans: Plan[];
  canViewAudit: boolean;
}

type FeedItem =
  | { kind: 'job'; id: string; timestamp: number; job: Job }
  | { kind: 'audit'; id: string; timestamp: number; event: AuditEvent };

const stateMeta: Record<JobState, { icon: string; label: string }> = {
  pending: { icon: '○', label: 'Pending' },
  running: { icon: '●', label: 'Running' },
  succeeded: { icon: '✓', label: 'Succeeded' },
  failed: { icon: '!', label: 'Failed' },
  cancelled: { icon: '×', label: 'Cancelled' },
  rollback_prepared: { icon: '↶', label: 'Rollback prepared' },
  rolled_back: { icon: '↩', label: 'Rolled back' },
};

function timeValue(value?: string): number {
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function jobTimestamp(job: Job): number {
  return Math.max(
    timeValue(job.finished_at),
    timeValue(job.started_at),
    ...job.steps.flatMap((step) => [timeValue(step.finished_at), timeValue(step.started_at)]),
  );
}

function formatTimestamp(value?: string): string {
  const timestamp = timeValue(value);
  return timestamp ? new Date(timestamp).toLocaleString() : 'Not started';
}

function latestJobTimestamp(job: Job): string | undefined {
  return [
    job.finished_at,
    job.started_at,
    ...job.steps.flatMap((step) => [step.finished_at, step.started_at]),
  ].filter((value): value is string => Boolean(value)).sort((a, b) => timeValue(b) - timeValue(a))[0];
}

function StateBadge({ state }: { state: JobState }) {
  const meta = stateMeta[state];
  return <span className={`activity-state activity-state--${state}`}><span aria-hidden="true">{meta.icon}</span> {meta.label}</span>;
}

export function ActivityFeed({ plans, canViewAudit }: Props) {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [auditEvents, setAuditEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [pollingPaused, setPollingPaused] = useState(false);
  const [pollError, setPollError] = useState('');
  const [auditUnavailable, setAuditUnavailable] = useState(!canViewAudit);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [tabHidden, setTabHidden] = useState(document.hidden);
  const timerRef = useRef<number | null>(null);
  const failuresRef = useRef(0);
  const stoppedRef = useRef(false);

  const loadAudit = useCallback(async () => {
    if (!canViewAudit || auditUnavailable) return;
    try {
      setAuditEvents(await api.listAudit());
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 403) {
        setAuditUnavailable(true);
        setAuditEvents([]);
      }
      // Job activity remains useful when audit history is unavailable.
    }
  }, [auditUnavailable, canViewAudit]);

  const poll = useCallback(async () => {
    if (stoppedRef.current || document.hidden) return;
    try {
      setJobs(await api.listJobs());
      await loadAudit();
      failuresRef.current = 0;
      setPollError('');
      setPollingPaused(false);
      setUpdatedAt(new Date());
    } catch (error) {
      failuresRef.current += 1;
      setPollError(error instanceof Error ? error.message : String(error));
      if (failuresRef.current >= MAX_POLL_FAILURES) {
        setPollingPaused(true);
        return;
      }
    } finally {
      setLoading(false);
    }

    if (!stoppedRef.current && !document.hidden) {
      const backoff = POLL_INTERVAL_MS * 2 ** Math.max(0, failuresRef.current - 1);
      timerRef.current = window.setTimeout(() => void poll(), backoff);
    }
  }, [loadAudit]);

  const retry = useCallback(() => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    failuresRef.current = 0;
    stoppedRef.current = false;
    setPollingPaused(false);
    setPollError('');
    setLoading(true);
    void poll();
  }, [poll]);

  useEffect(() => {
    stoppedRef.current = false;
    void poll();
    const handleVisibility = () => {
      const hidden = document.hidden;
      setTabHidden(hidden);
      if (timerRef.current !== null) {
        window.clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      if (!hidden) retry();
    };
    document.addEventListener('visibilitychange', handleVisibility);
    return () => {
      stoppedRef.current = true;
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
      document.removeEventListener('visibilitychange', handleVisibility);
    };
  }, [poll, retry]);

  const refreshJob = async (jobID: string) => {
    try {
      const detail = await api.getJob(jobID);
      setJobs((current) => current.map((job) => (job.id === detail.id ? detail : job)));
    } catch {
      // The next successful poll reconciles a transient detail-fetch failure.
    }
  };

  const feed = useMemo<FeedItem[]>(() => [
    ...jobs.map((job): FeedItem => ({ kind: 'job', id: `job:${job.id}`, timestamp: jobTimestamp(job), job })),
    ...auditEvents.map((event): FeedItem => ({ kind: 'audit', id: `audit:${event.id}`, timestamp: timeValue(event.timestamp), event })),
  ].sort((a, b) => b.timestamp - a.timestamp || a.id.localeCompare(b.id)), [auditEvents, jobs]);

  return (
    <section className="activity-feed" aria-label="Migration activities">
      <div className="activity-toolbar">
        <div>
          <strong>Live activities</strong>
          <div className="meta">
            {tabHidden ? 'Polling paused while this tab is hidden' : pollingPaused ? 'Polling paused after repeated errors' : 'Updates every 5 seconds'}
            {updatedAt ? ` · Last updated ${updatedAt.toLocaleTimeString()}` : ''}
          </div>
        </div>
        <button className="btn activity-refresh" type="button" onClick={retry} disabled={loading || tabHidden}>
          {loading ? <span className="spinner" /> : pollingPaused ? 'Retry' : 'Refresh'}
        </button>
      </div>

      {pollError && <div className="activity-notice error-text">Activity refresh failed{pollingPaused ? ' repeatedly; automatic polling is paused.' : '; retrying with backoff.'} {pollError}</div>}
      {auditUnavailable && <div className="activity-notice meta">Audit entries are hidden for this role. Job activity remains available.</div>}

      <div className="activity-timeline">
        {feed.map((item) => item.kind === 'job' ? (
          <JobActivity key={item.id} job={item.job} plan={plans.find((plan) => plan.id === item.job.plan_id)} onExpand={refreshJob} />
        ) : <AuditActivity key={item.id} event={item.event} />)}
        {!loading && feed.length === 0 && <div className="empty">No migration activity yet.</div>}
      </div>
    </section>
  );
}

function JobActivity({ job, plan, onExpand }: { job: Job; plan?: Plan; onExpand: (jobID: string) => void }) {
  const timestamp = latestJobTimestamp(job);
  return (
    <details className={`activity-entry activity-entry--job activity-entry--${job.state}`} onToggle={(event) => {
      if (event.currentTarget.open) void onExpand(job.id);
    }}>
      <summary>
        <span className="activity-dot" aria-hidden="true" />
        <span className="activity-summary-copy">
          <span className="activity-title">{plan?.name ?? job.plan_id}</span>
          <span className="meta">Job {job.id} · {formatTimestamp(timestamp)}</span>
        </span>
        <StateBadge state={job.state} />
      </summary>
      <div className="activity-details">
        {job.steps.map((step, index) => (
          <div className={`activity-step-row activity-step-row--${step.state}`} key={step.id ?? `${job.id}-${step.name}-${index}`}>
            <span className="activity-step-marker" aria-hidden="true" />
            <div>
              <div className="activity-step-title"><span>{step.name}</span><StateBadge state={step.state} /></div>
              <div className="meta">Started: {formatTimestamp(step.started_at)}{step.finished_at ? ` · Finished: ${formatTimestamp(step.finished_at)}` : ''}</div>
              {step.message && <div className="activity-message">{step.message}</div>}
            </div>
          </div>
        ))}
        {job.steps.length === 0 && <div className="empty">No job steps have been recorded.</div>}
      </div>
    </details>
  );
}

function AuditActivity({ event }: { event: AuditEvent }) {
  return (
    <article className="activity-entry activity-entry--audit">
      <span className="activity-dot" aria-hidden="true" />
      <div className="activity-summary-copy">
        <div className="activity-title">{event.action}</div>
        <div className="meta">{event.actor} · {event.target} · {formatTimestamp(event.timestamp)}</div>
        {event.detail && <div className="activity-message">{event.detail}</div>}
      </div>
      <span className="activity-audit-result">{event.result}</span>
    </article>
  );
}
