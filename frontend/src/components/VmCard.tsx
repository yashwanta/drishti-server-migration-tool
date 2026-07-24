import type { VM } from '../types';

interface Props {
  vm: VM;
  onDragStart: (vm: VM) => void;
  onDragEnd: () => void;
  dragging: boolean;
}

function fmtBytes(bytes: number): string {
  const gb = bytes / (1024 * 1024 * 1024);
  if (gb >= 1024) return `${(gb / 1024).toFixed(1)} TB`;
  return `${gb.toFixed(0)} GB`;
}

export function VmCard({ vm, onDragStart, onDragEnd, dragging }: Props) {
  const familyIcon = vm.guest_family === 'windows' ? 'W' : vm.guest_family === 'linux' ? 'L' : '?';
  return (
    <div
      className={`vm-card ${vm.power_state} ${dragging ? 'dragging' : ''}`}
      draggable
      onDragStart={() => onDragStart(vm)}
      onDragEnd={onDragEnd}
      title={vm.notes}
    >
      <div className="name">
        [{familyIcon}] {vm.name}
      </div>
      <div className="meta">
        {vm.cpus} vCPU - {vm.memory_mb} MB - {vm.disks.length} disk(s) -{' '}
        {fmtBytes(vm.disks.reduce((a, d) => a + d.capacity_bytes, 0))}
      </div>
      <div className="meta">
        <span className={`badge ${vm.power_state === 'on' ? 'green' : ''}`}>{vm.power_state}</span>{' '}
        <span className="badge">{vm.firmware}</span>{' '}
        {vm.snapshots.length > 0 && <span className="badge amber">{vm.snapshots.length} snap</span>}
      </div>
    </div>
  );
}