import { useState } from 'react';
import type { TargetNode } from '../types';

interface Props {
  node: TargetNode;
  onDrop: (node: TargetNode) => void;
}

function fmtBytes(bytes: number): string {
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
  return `${gb.toFixed(0)} GB`;
}

export function NodeCard({ node, onDrop }: Props) {
  const [active, setActive] = useState(false);
  return (
    <div
      className={`node-card ${active ? 'drop-active' : ''} ${!node.online ? 'offline' : ''}`}
      onDragOver={(e) => {
        e.preventDefault();
        if (node.online) setActive(true);
      }}
      onDragLeave={() => setActive(false)}
      onDrop={(e) => {
        e.preventDefault();
        setActive(false);
        if (node.online) onDrop(node);
      }}
    >
      <div className="name">
        o {node.name} {!node.online && <span className="badge red">offline</span>}
      </div>
      <div className="caps">
        {(node.cpu_total_mhz - node.cpu_used_mhz).toLocaleString()} MHz free -{' '}
        {((node.memory_total_mb - node.memory_used_mb) / 1024).toFixed(0)} GB RAM free - next VMID {node.next_vmid}
      </div>
      <div className="caps">
        Storage: {(node.storage ?? []).map((s) => `${s.name} (${fmtBytes(s.free_bytes)} free)`).join(', ')}
      </div>
      <div className="caps">
        Bridges: {(node.bridges ?? []).map((b) => b.name + (b.vlan_aware ? ' (vlan)' : '')).join(', ')}
      </div>
      <div className="target-vms">
        <div className="target-vms-title">Virtual machines ({(node.vms ?? []).length})</div>
        {(node.vms ?? []).map((vm) => (
          <div className="target-vm" key={vm.id}>
            <span className="target-vm-name">{vm.id} — {vm.name}</span>
            <span className={`badge ${vm.status === 'on' ? 'green' : ''}`}>{vm.status}</span>
            <span className="caps">{vm.cpus} vCPU · {(vm.memory_mb / 1024).toFixed(1)} GB RAM</span>
          </div>
        ))}
        {(node.vms ?? []).length === 0 && <div className="empty">No VMs on this node.</div>}
      </div>
    </div>
  );
}
