import { useCallback, useEffect, useState } from 'react';
import type { Connection, InventoryRoot, Plan, TargetNode, VM } from './types';
import { api } from './api';
import { VmCard } from './components/VmCard';
import { NodeCard } from './components/NodeCard';
import { PlanWizard } from './components/PlanWizard';

interface DropTarget {
  vm: VM;
  node: TargetNode;
  sourceConnId: string;
  targetConnId: string;
}

export default function App() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [vmwareInv, setVmwareInv] = useState<InventoryRoot | null>(null);
  const [proxmoxInv, setProxmoxInv] = useState<InventoryRoot | null>(null);
  const [plans, setPlans] = useState<Plan[]>([]);
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const conns = await api.listConnections();
      setConnections(conns);
      const source = conns.find((c) => c.role === 'source');
      const target = conns.find((c) => c.role === 'target');
      if (source) setVmwareInv(await api.getInventory(source.id));
      if (target) setProxmoxInv(await api.getInventory(target.id));
      setPlans(await api.listPlans());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const handleDrop = useCallback(
    (node: TargetNode) => {
      if (!draggingId || !vmwareInv) return;
      const vm = vmwareInv.datacenters
        ?.flatMap((dc) => dc.clusters.flatMap((cl) => cl.hosts.flatMap((h) => h.vms)))
        .find((v) => v.id === draggingId);
      if (!vm) return;
      const sourceConn = connections.find((c) => c.role === 'source');
      const targetConn = connections.find((c) => c.role === 'target');
      if (!sourceConn || !targetConn) return;
      // SAFETY: dropping only opens a plan wizard. It never migrates.
      setDropTarget({ vm, node, sourceConnId: sourceConn.id, targetConnId: targetConn.id });
    },
    [draggingId, vmwareInv, connections],
  );

  const allVms: VM[] =
    vmwareInv?.datacenters?.flatMap((dc) => dc.clusters.flatMap((cl) => cl.hosts.flatMap((h) => h.vms))) ?? [];

  return (
    <div className="app">
      <div className="topbar">
        <h1>DRISHTI HyperShift</h1>
        <span className="mode">MOCK MODE</span>
        <span style={{ flex: 1 }} />
        <button className="btn" onClick={() => void load()} disabled={loading}>
          {loading ? <span className="spinner" /> : 'Refresh'}
        </button>
      </div>

      <div className="banner">
        Safety notice: Dragging a VM only opens a migration plan. No migration runs automatically.
        The source VM stays registered in VMware for rollback. Source and target must never run
        simultaneously on the same production network.
      </div>

      {error && <div className="banner" style={{ borderColor: 'var(--red)', color: '#ffb4b0' }}>{error}</div>}

      <div className="content">
        <div className="col">
          <h2>VMware Source</h2>
          {vmwareInv?.datacenters?.map((dc) => (
            <div key={dc.id} className="panel">
              <header>{dc.name}</header>
              <div className="body">
                {dc.clusters.flatMap((cl) =>
                  cl.hosts.flatMap((h) => (
                    <div key={h.id} style={{ marginBottom: 12 }}>
                      <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 6 }}>{h.name}</div>
                      {h.vms.map((vm) => (
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
                )}
              </div>
            </div>
          ))}
          {!loading && allVms.length === 0 && <div className="empty">No source VMs found.</div>}
        </div>

        <div className="col">
          <h2>Proxmox Target</h2>
          {proxmoxInv?.nodes?.map((node) => (
            <NodeCard key={node.id} node={node} onDrop={handleDrop} />
          ))}
          {!loading && (proxmoxInv?.nodes?.length ?? 0) === 0 && (
            <div className="empty">No target nodes found.</div>
          )}
        </div>

        <div className="col">
          <h2>Draft Plans ({plans.length})</h2>
          <div className="panel">
            <div className="body">
              {plans.length === 0 && <div className="empty">No plans yet. Drag a VM onto a Proxmox node.</div>}
              {plans.map((p) => (
                <div key={p.id} style={{ padding: '8px 0', borderBottom: '1px solid var(--border)' }}>
                  <div style={{ fontWeight: 600, fontSize: 13 }}>{p.name}</div>
                  <div className="meta" style={{ fontSize: 12, color: 'var(--muted)' }}>
                    {p.source_vm_id} {'->'} {p.target_node_id} Â·{' '}
                    <span className="badge amber">{p.status}</span>
                  </div>
                </div>
              ))}
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
          onClose={() => setDropTarget(null)}
          onCreated={(plan) => {
            setPlans((prev) => [plan, ...prev]);
            setDropTarget(null);
            void load();
          }}
        />
      )}
    </div>
  );
}