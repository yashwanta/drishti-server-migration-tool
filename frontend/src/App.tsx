import { useCallback, useEffect, useState } from 'react';
import type { Connection, InventoryRoot, Job, Plan, TargetNode, VM, ConnRole, PlatformKind } from './types';
import { api } from './api';
import { VmCard } from './components/VmCard';
import { NodeCard } from './components/NodeCard';
import { PlanWizard } from './components/PlanWizard';
import { AddConnectionModal } from './components/AddConnectionModal';
import { MigrationPanel } from './components/MigrationPanel';

interface DropTarget {
  vm: VM;
  node: TargetNode;
  sourceConnId: string;
  targetConnId: string;
  sourceKind: PlatformKind;
}

function arr<T>(v: T[] | null | undefined): T[] {
  return Array.isArray(v) ? v : [];
}

export default function App() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [inventory, setInventory] = useState<Record<string, InventoryRoot>>({});
  const [inventoryErrors, setInventoryErrors] = useState<Record<string, string>>({});
  const [mode, setMode] = useState('unknown');
  const [plans, setPlans] = useState<Plan[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
  const [addModal, setAddModal] = useState<ConnRole | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const conns = await api.listConnections();
      setConnections(conns);
      const inv: Record<string, InventoryRoot> = {};
      const invErrors: Record<string, string> = {};
      await Promise.all(
        conns.map(async (c) => {
          try {
            inv[c.id] = await api.getInventory(c.id);
          } catch (e) {
            invErrors[c.id] = e instanceof Error ? e.message : String(e);
          }
        }),
      );
      setInventory(inv);
      setInventoryErrors(invErrors);
      setPlans(await api.listPlans());
      setJobs(await api.listJobs());
      try {
        const health = await api.health();
        setMode(health.mode);
      } catch {
        setMode('unknown');
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void api.listJobs().then(setJobs).catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(timer);
  }, []);

  const sources = connections.filter((c) => c.role === 'source');
  const targets = connections.filter((c) => c.role === 'target');

  const handleDrop = useCallback(
    (node: TargetNode, targetConnId: string) => {
      if (!draggingId) return;
      // Find the VM across all source inventories.
      for (const inv of Object.values(inventory)) {
        const vm = arr(inv.datacenters)
          .flatMap((dc) => arr(dc.clusters).flatMap((cl) => arr(cl.hosts).flatMap((h) => arr(h.vms))))
          .find((v) => v.id === draggingId);
        if (vm) {
          const sourceConn = sources.find((c) => c.id === inv.connection_id);
          const targetConn = connections.find((c) => c.id === targetConnId);
          if (sourceConn && targetConn) {
            setDropTarget({ vm, node, sourceConnId: sourceConn.id, targetConnId: targetConn.id, sourceKind: sourceConn.kind });
          }
          return;
        }
      }
    },
    [draggingId, inventory, sources, connections],
  );

  const deleteConn = async (id: string) => {
    if (!confirm('Remove this connection? Existing draft plans are not affected.')) return;
    try {
      await api.deleteConnection(id);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="app">
      <div className="topbar">
        <img className="brand-logo" src="/drishti-hypershift-logo-v3.png" alt="DRISHTI HyperShift" />
        <span className="mode">{mode.toUpperCase()} MODE</span>
        <span style={{ flex: 1 }} />
        <button className="btn" onClick={() => void load()} disabled={loading}>
          {loading ? <span className="spinner" /> : 'Refresh'}
        </button>
      </div>

      <div className="banner">
        Safety notice: Dragging a VM only opens a migration plan. No migration runs automatically.
        The source VM stays registered for rollback. Source and target must never run simultaneously
        on the same production network.
      </div>

      {error && <div className="banner" style={{ borderColor: 'var(--red)', color: '#ffb4b0' }}>{error}</div>}

      <div className="content">
        {/* SOURCE PANEL */}
        <div className="col">
          <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
            <h2 style={{ margin: 0 }}>VMware / Source</h2>
            <span style={{ flex: 1 }} />
            <button className="btn primary" onClick={() => setAddModal('source')}>+ Add Source</button>
          </div>
          {sources.map((conn) => {
            const inv = inventory[conn.id];
            return (
              <div key={conn.id} className="panel">
                <header>
                  <span style={{ flex: 1 }}>{conn.name}</span>
                  <span className={`badge ${conn.status === 'connected' ? 'green' : 'red'}`}>{conn.status}</span>
                  <button
                    className="btn"
                    style={{ padding: '2px 8px', fontSize: 12, marginLeft: 8 }}
                    onClick={() => void deleteConn(conn.id)}
                    title="Remove connection"
                  >
                    x
                  </button>
                </header>
                <div className="body">
                  {conn.endpoint && (
                    <div className="meta" style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 6 }}>
                      {conn.kind} - {conn.endpoint}
                    </div>
                  )}
                  {arr(inv?.datacenters).map((dc) =>
                    arr(dc.clusters).flatMap((cl) =>
                      arr(cl.hosts).flatMap((h) => (
                        <div key={h.id} style={{ marginBottom: 12 }}>
                          <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 6 }}>{h.name}</div>
                          {arr(h.vms).map((vm) => (
                            <VmCard
                              key={vm.id}
                              vm={vm}
                              dragging={draggingId === vm.id}
                              onDragStart={(v) => setDraggingId(v.id)}
                              onDragEnd={() => setDraggingId(null)}
                            />
                          ))}
                        </div>
                      )),
                    ),
                  )}
                  {inv && arr(inv.datacenters).length === 0 && <div className="empty">No VMs in this inventory.</div>}
                  {inventoryErrors[conn.id] && <div className="error-text">{inventoryErrors[conn.id]}</div>}
                  {!inv && !inventoryErrors[conn.id] && <div className="empty">Loading inventory...</div>}
                </div>
              </div>
            );
          })}
          {sources.length === 0 && !loading && (
            <div className="empty">No sources. Click "+ Add Source" to connect vCenter or ESXi.</div>
          )}
        </div>

        {/* TARGET PANEL */}
        <div className="col">
          <div style={{ display: 'flex', alignItems: 'center', marginBottom: 4 }}>
            <h2 style={{ margin: 0 }}>Proxmox / Target</h2>
            <span style={{ flex: 1 }} />
            <button className="btn primary" onClick={() => setAddModal('target')}>+ Add Target</button>
          </div>
          {targets.map((conn) => {
            const inv = inventory[conn.id];
            return (
              <div key={conn.id} className="panel">
                <header>
                  <span style={{ flex: 1 }}>{conn.name}</span>
                  <span className={`badge ${conn.status === 'connected' ? 'green' : 'red'}`}>{conn.status}</span>
                  <button
                    className="btn"
                    style={{ padding: '2px 8px', fontSize: 12, marginLeft: 8 }}
                    onClick={() => void deleteConn(conn.id)}
                    title="Remove connection"
                  >
                    x
                  </button>
                </header>
                <div className="body">
                  {conn.endpoint && (
                    <div className="meta" style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 6 }}>
                      {conn.kind} - {conn.endpoint}
                    </div>
                  )}
                  {arr(inv?.nodes).map((node) => (
                    <NodeCard key={node.id} node={node} onDrop={(n) => handleDrop(n, conn.id)} />
                  ))}
                  {inv && arr(inv.nodes).length === 0 && <div className="empty">No nodes in this inventory.</div>}
                  {inventoryErrors[conn.id] && <div className="error-text">{inventoryErrors[conn.id]}</div>}
                  {!inv && !inventoryErrors[conn.id] && <div className="empty">Loading inventory...</div>}
                </div>
              </div>
            );
          })}
          {targets.length === 0 && !loading && (
            <div className="empty">No targets. Click "+ Add Target" to connect Proxmox VE.</div>
          )}
        </div>

        {/* PLANS PANEL */}
        <div className="col">
          <h2>Migration Plans ({plans.length})</h2>
          <div className="panel">
            <div className="body">
              <MigrationPanel plans={plans} onRefresh={() => void load()} />
            </div>
          </div>
          <h2 style={{ marginTop: 14 }}>Activity ({jobs.length})</h2>
          <div className="panel">
            <div className="body activity-list">
              {jobs.map((job) => {
                const plan = plans.find((item) => item.id === job.plan_id);
                const message = job.steps[0]?.message ?? '';
                const match = message.match(/([0-9]+(?:\.[0-9]+)?)%/);
                const progress = match ? Math.min(100, Math.max(0, Number(match[1]))) : null;
                return (
                  <div className="activity-item" key={job.id}>
                    <div className="activity-head">
                      <span>{plan?.name ?? job.plan_id}</span>
                      <span className={`badge ${job.state === 'succeeded' ? 'green' : job.state === 'failed' ? 'red' : 'amber'}`}>{job.state}</span>
                    </div>
                    <div className="meta">{plan?.source_vm_id ?? 'migration'} → VMID {plan?.target_vmid ?? 'pending'} on {plan?.target_node_id ?? 'target'}</div>
                    {job.state === 'running' && progress !== null && (
                      <div className="progress-wrap" aria-label={`Migration ${progress.toFixed(1)} percent complete`}>
                        <div className="progress-bar"><div className="progress-fill" style={{ width: `${progress}%` }} /></div>
                        <span>{progress.toFixed(1)}%</span>
                      </div>
                    )}
                    {job.steps.map((step, index) => (
                      <div className="activity-step" key={step.id ?? `${job.id}-${index}`}>
                        {step.name}: {step.state}{step.message ? ` — ${step.message}` : ''}
                      </div>
                    ))}
                    <div className="meta">
                      {job.started_at ? new Date(job.started_at).toLocaleString() : ''}
                      {job.finished_at ? ` – ${new Date(job.finished_at).toLocaleString()}` : ''}
                    </div>
                  </div>
                );
              })}
              {jobs.length === 0 && <div className="empty">No migration activity yet.</div>}
            </div>
          </div>
        </div>
      </div>

      {dropTarget && (
        <PlanWizard
          vm={dropTarget.vm}
          node={dropTarget.node}
          sourceConnId={dropTarget.sourceConnId}
          targetConnId={dropTarget.targetConnId}
          sourceKind={dropTarget.sourceKind}
          labMode={mode === 'lab'}
          onClose={() => setDropTarget(null)}
          onCreated={(plan) => {
            setPlans((prev) => [plan, ...prev]);
            setDropTarget(null);
            void load();
          }}
        />
      )}

      {addModal && (
        <AddConnectionModal
          defaultRole={addModal}
          onClose={() => setAddModal(null)}
          onCreated={() => {
            setAddModal(null);
            void load();
          }}
        />
      )}
    </div>
  );
}
