# Worker qemu-img Conversion Lab Runbook

Use this procedure only with disposable VMDK data in an isolated lab workspace.
Real-mode migration execution remains locked at HTTP 501.

## Configuration

The backend requires the worker endpoint in every non-mock mode:

```text
DRISHTI_WORKER_URL=http://worker:8090
```

The worker conversion endpoint is disabled by default. It can be enabled only
in lab or live mode:

```text
DRISHTI_MODE=lab
DRISHTI_ENABLE_WORKER_CONVERSION=true
DRISHTI_WORKSPACE_ROOT=/mnt/drishti-import
DRISHTI_MAX_WORKSPACE_GB=100
DRISHTI_CONVERSION_TIMEOUT=24h
```

Production mode rejects worker conversion enablement. The worker container runs
as UID 10001 and includes `qemu-img` from Debian's `qemu-utils` package.

## Enforced boundary

- Only `qemu-img convert -f vmdk -O raw|qcow2 <input> <partial-output>` is
  accepted by the command-specific safelist.
- No shell is invoked and no command string is assembled.
- Input and output must resolve beneath the worker workspace.
- Input, output, and stale partial files must be regular, non-symlink files.
- The input is checksummed before and after conversion to detect concurrent
  modification.
- Output is published by atomic rename only after qemu-img succeeds.
- A sidecar manifest binds the input checksum, output checksum, target format,
  and size. A retry reuses an output only when all evidence matches.
- Workspace size caps and request cancellation/timeouts are enforced.

## Disposable qualification

1. Mount a dedicated workspace shared only by the worker and disposable target.
2. Place a powered-off lab VM's exported VMDK beneath that workspace.
3. Submit the typed conversion request to `POST /api/v1/conversions`.
4. Confirm the result includes format, size, SHA-256, duration, and `reused=false`.
5. Run `qemu-img info` independently and confirm the requested format and virtual size.
6. Repeat the request and confirm `reused=true` with the same SHA-256.
7. Corrupt either the output or manifest and confirm reuse is refused.
8. Confirm the durable `convert_disks` job step contains checksum evidence.
9. Disable conversion and remove all disposable artifacts.

Do not use a production VMDK or production-mounted workspace for qualification.
