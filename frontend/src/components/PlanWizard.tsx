import { useState } from 'react';
import type { Plan, TargetNode, VM, Firmware } from '../types';
import { api } from '../api';

interface Props {
  vm: VM;
  node: TargetNode;
  sourceConnId: string;
  targetConnId: string;
  onClose: () => void;
  onCreated: (plan: Plan) => void;
}

type Step = 0 | 1 | 2 | 3;
const STEP_NAMES = ['Source', 'Target', 'Mappings', 'Review'];

export function PlanWizard({ vm, node, sourceConnId, targetConnId, onClose, onCreated }: Props) {
  const [step, setStep] = useState<Step>(0);
  const [cpu, setCpu] = useState(vm.cpus);
  const [memory, setMemory] = useState(vm.memory_mb);
  const [firmware, setFirmware] = useState<Firmware>(vm.firmware);
  const [targetName, setTargetName] = useState(vm.name);
  const [diskFormat, setDiskFormat] = useState<'raw' | 'qcow2'>('qcow2');
  const [storageMaps, setStorageMaps] = useState<Record<string, string>>(
    Object.fromEntries(vm.disks.map((d) => [d.id, node.storage[0]?.id ?? ''])),
  );
  const [networkMaps, setNetworkMaps] = useState<Record<string, string>>(
    Object.fromEntries(vm.nics.map((n) => [n.id, node.bridges[0]?.name ?? ''])),
  );
  const [vlanId, setVlanId] = useState<number>(0);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const next = () => setStep((s) => (s < 3 ? ((s + 1) as Step) : s));
  const back = () => setStep((s) => (s > 0 ? ((s - 1) as Step) : s));

  const submit = async () => {
    setSubmitting(true);
    setError('');
    try {
      const plan = await api.createPlan({
        name: `migrate-${vm.name}-to-${node.name}`,
        source_vm_id: vm.id,
        source_connection_id: sourceConnId,
        target_node_id: node.id,
        target_connection_id: targetConnId,
        target_vm_name: targetName,
        cpu,
        memory_mb: memory,
        firmware,
        disk_format: diskFormat,
        storage_maps: vm.disks.map((d) => ({
          source_disk_id: d.id,
          target_storage_id: storageMaps[d.id] ?? '',
          target_format: diskFormat,
        })),
        network_maps: vm.nics.map((n) => ({
          source_nic_id: n.id,
          target_bridge: networkMaps[n.id] ?? '',
          vlan_id: vlanId || undefined,
        })),
      });
      onCreated(plan);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header>{'Migration Plan: ' + vm.name + ' -> ' + node.name}</header>
        <div className="modal-body">
          <div className="steps">
            {STEP_NAMES.map((n, i) => (
              <div key={n} className={`step-pill ${i === step ? 'active' : i < step ? 'done' : ''}`}>
                {i + 1}. {n}
              </div>
            ))}
          </div>

          {step === 0 && (
            <div>
              <div className="summary-row"><span>VM Name</span><span className="val">{vm.name}</span></div>
              <div className="summary-row"><span>Guest OS</span><span className="val">{vm.guest_os}</span></div>
              <div className="summary-row"><span>Power State</span><span className="val">{vm.power_state}</span></div>
              <div className="summary-row"><span>vCPU / Memory</span><span className="val">{vm.cpus} / {vm.memory_mb} MB</span></div>
              <div className="summary-row"><span>Disks</span><span className="val">{vm.disks.length}</span></div>
              <div className="summary-row"><span>NICs</span><span className="val">{vm.nics.length}</span></div>
              <div className="summary-row"><span>Snapshots</span><span className="val">{vm.snapshots.length}</span></div>
            </div>
          )}

          {step === 1 && (
            <div>
              <div className="field">
                <label>Target VM Name</label>
                <input value={targetName} onChange={(e) => setTargetName(e.target.value)} />
              </div>
              <div className="field">
                <label>CPU (cores)</label>
                <input type="number" min={1} value={cpu} onChange={(e) => setCpu(parseInt(e.target.value) || 1)} />
              </div>
              <div className="field">
                <label>Memory (MB)</label>
                <input type="number" min={128} value={memory} onChange={(e) => setMemory(parseInt(e.target.value) || 512)} />
              </div>
              <div className="field">
                <label>Firmware</label>
                <select value={firmware} onChange={(e) => setFirmware(e.target.value as Firmware)}>
                  <option value="bios">BIOS</option>
                  <option value="uefi">UEFI</option>
                </select>
              </div>
            </div>
          )}

          {step === 2 && (
            <div>
              <label style={{ fontSize: 12, color: 'var(--muted)' }}>Storage Mapping</label>
              {vm.disks.map((d) => (
                <div className="field" key={d.id}>
                  <label>{d.label} ({(d.capacity_bytes / (1024 ** 3)).toFixed(0)} GB)</label>
                  <select value={storageMaps[d.id] ?? ''} onChange={(e) => setStorageMaps({ ...storageMaps, [d.id]: e.target.value })}>
                    {node.storage.map((s) => (
                      <option key={s.id} value={s.id}>{s.name} ({s.type}, {(s.free_bytes / (1024 ** 3)).toFixed(0)} GB free)</option>
                    ))}
                  </select>
                </div>
              ))}
              <div className="field">
  <label>Target Disk Format</label>
  <select value={diskFormat} onChange={(e) => setDiskFormat(e.target.value as 'raw' | 'qcow2')}>
    <option value="qcow2">qcow2</option>
    <option value="raw">raw</option>
  </select>
</div>
<label style={{ fontSize: 12, color: 'var(--muted)' }}>Network Mapping</label>
              {vm.nics.map((n) => (
                <div className="field" key={n.id}>
                  <label>{n.label}</label>
                  <select value={networkMaps[n.id] ?? ''} onChange={(e) => setNetworkMaps({ ...networkMaps, [n.id]: e.target.value })}>
                    {node.bridges.map((b) => (
                      <option key={b.name} value={b.name}>{b.name}{b.vlan_aware ? ' (VLAN-aware)' : ''}</option>
                    ))}
                  </select>
                </div>
              ))}
              <div className="field">
                <label>Target VLAN ID (0 = none)</label>
                <input type="number" min={0} max={4094} value={vlanId} onChange={(e) => setVlanId(parseInt(e.target.value) || 0)} />
              </div>
            </div>
          )}

          {step === 3 && (
            <div>
              <div className="summary-row"><span>Source</span><span className="val">{vm.name} ({vm.guest_os})</span></div>
              <div className="summary-row"><span>Target</span><span className="val">{node.name} / {targetName}</span></div>
              <div className="summary-row"><span>CPU / Memory</span><span className="val">{cpu} / {memory} MB</span></div>
              <div className="summary-row"><span>Firmware</span><span className="val">{firmware}</span></div>
              <div className="summary-row"><span>Disk Format</span><span className="val">{diskFormat}</span></div>
              <div className="summary-row"><span>Storage Maps</span><span className="val">{vm.disks.length}</span></div>
              <div className="summary-row"><span>Network Maps</span><span className="val">{vm.nics.length}</span></div>
              <div className="warn-box">
                Submitting creates a DRAFT plan only. No migration, disk copy, power change, or deletion
                will occur. The source VM remains registered on its source platform for rollback.
              </div>
            </div>
          )}

          {error && <div className="error-text">{error}</div>}
        </div>
        <footer>
          <button className="btn" onClick={onClose}>Cancel</button>
          {step > 0 && <button className="btn" onClick={back}>Back</button>}
          {step < 3 && <button className="btn primary" onClick={next}>Next</button>}
          {step === 3 && (
            <button className="btn primary" onClick={submit} disabled={submitting}>
              {submitting ? <span className="spinner" /> : 'Create Draft Plan'}
            </button>
          )}
        </footer>
      </div>
    </div>
  );
}
