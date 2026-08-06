# VMware Source Export Lab Runbook

This runbook applies only to a disposable VMware lab VM. Item 0 is not wired
into the non-mock job factory until item 2, and production-mode mutation is
prohibited in code.

## Safety boundary

- DRISHTI may read inventory and export a supported VMDK from a confirmed
  powered-off VM.
- DRISHTI may power off one named VM only when the approved plan separately
  authorizes that exact source VM and the platform-mutation interlock is set.
- DRISHTI never powers on, deletes, unregisters, reconfigures, relocates, or
  consolidates snapshots on the VMware source.
- A source with snapshots/delta backing, RDM, encryption, an inaccessible
  datastore, an unknown/suspended power state, or an unsupported disk backing
  is refused.
- Rollback isolates and stops the target, then requires an authorized VMware
  operator to start and validate the retained source manually.

## Lab prerequisites

1. Use a disposable VM on a non-production vCenter or standalone ESXi host.
2. Disconnect it from production networks and confirm its hostname, IP, MAC,
   and application identity cannot conflict with production.
3. Remove snapshots manually before testing. Do not ask DRISHTI to consolidate
   them.
4. Use a least-privilege VMware account with inventory/datastore read access
   and, only when power-off testing is required, permission to power off the
   disposable VM. It needs no delete, unregister, reconfigure, relocate,
   snapshot-removal, or power-on privilege.
5. Set a dedicated worker workspace with enough free space for the descriptor
   and all VMDK extents.

## Interlocks

Use `DRISHTI_MODE=lab` for qualification. `live` is accepted by the shared
policy but must not be used until the lab qualification gate is signed off.

Set `DRISHTI_ENABLE_PLATFORM_MUTATION=true` only for the controlled power-off
test. Leave it false when the VM is already powered off. Add every protected
vCenter/ESXi endpoint to `DRISHTI_MUTATION_DENYLIST`.

`DRISHTI_MODE=production` rejects platform mutation even if the mutation flag
is mistakenly set.

## Test sequence

1. Confirm the disposable VM is registered and record its managed-object ID.
2. Run inventory and verify the selected disk device key and datastore.
3. Prefer powering off the VM manually. Confirm DRISHTI reports it as off.
4. If testing controlled power-off, approve the plan and the separate source
   power-off confirmation for that exact VM, enable the mutation flag, and
   verify the resulting VMware task reaches `poweredOff`.
5. Export one supported persistent flat VMDK. Verify the descriptor, every
   referenced extent, the manifest, sizes, and SHA-256 values beneath the
   dedicated workspace.
6. Confirm the VM remains registered and powered off and that no source
   configuration or snapshot changed.
7. Disable the mutation flag immediately after the test.

Do not use this procedure against production infrastructure. A lab integration
test is intentionally not run until the operator provides and explicitly
identifies a disposable lab endpoint and VM.
