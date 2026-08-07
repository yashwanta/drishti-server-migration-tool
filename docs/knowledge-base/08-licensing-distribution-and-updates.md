# Licensing, Distribution, and Product Updates

> **Status:** Product design approved; licensing and installer enforcement are
> not implemented yet. This article records the intended commercial behavior so
> implementation, support, and sales use the same rules.

## Product tiers

DRISHTI HyperShift will be distributed in two licensed tiers. License files are
signed by DRISHTI and validated locally. The installed product contains only a
public verification key; the private signing key is never included.

### Trial

A trial installation is limited to:

- One activated machine and local subnet.
- One source VM ID, recorded when the first migration plan is created.
- One reserved migration plan that may eventually produce one completed job.
- Thirty calendar days from first activation.

The effective expiration is the earlier of the signed license deadline and 30
days after first activation:

```text
effective_expiry = min(signed_not_after, first_activation + 30 days)
```

Expiration never deletes plans, jobs, audit records, or customer data. It
blocks licensed operations and leaves license status/import functions
available so the customer can purchase and activate a full license.

Reinstalling the application does not reset a trial. Activation state includes
the machine fingerprint, subnet, DNS suffix, activation time, highest observed
clock, bound source VM, and execution reservation.

### Full

A full license removes trial VM, completion, and time restrictions. It does not
bypass operational safety controls. Authentication, RBAC, preflight, approval,
`DRISHTI_MODE`, and mutation flags must still independently permit an action.

A perpetual full license keeps the purchased product version operational even
after its update entitlement ends.

## Update entitlements

Product use and access to new releases are separate rights:

- **One-time purchase:** perpetual use of the purchased version, without later
  feature releases.
- **Continuing update plan:** a signed license contains an `updates_until`
  date. Releases published on or before that date remain installable.
- **Renewal:** the customer imports a replacement signed license extending
  `updates_until`; reinstalling HyperShift is not required.

When update coverage has ended, HyperShift continues running and the updater
reports that the selected release requires an active update entitlement.

Updates will be available both online and as offline packages:

- Online: `https://www.drishtinoc.com/downloads/hypershift/`
- Offline: `DRISHTI-HyperShift-<version>.drishti-update`

Every package and version manifest must pass checksum and signature validation
before installation.

## Trial issuance and repeat-trial controls

Runtime license validation remains offline. Trial issuance uses a request file
submitted through the DRISHTI website:

1. The customer runs the shipped fingerprint utility.
2. The utility exports hashed machine, subnet, DNS, and installation values.
3. The customer submits the request through `www.drishtinoc.com`.
4. The issuance registry checks for prior trials.
5. DRISHTI issues a signed license that the product can validate offline.

The same machine or expired installation fingerprint is a hard rejection.
Public IP, DNS domain, and subnet are review signals because corporate NAT,
DHCP, VPNs, and shared networks can cause unrelated customers to appear alike.
Public IP alone must not automatically reject a legitimate customer.

## Machine and subnet binding

The machine fingerprint is derived from a stable operating-system identifier:

- Windows: Machine GUID.
- Linux: `/etc/machine-id`.

Raw identifiers are hashed and are never returned by the license status API or
written to logs.

Subnet binding uses an explicitly configured network interface when supplied.
Automatic selection is allowed only when exactly one eligible private,
non-loopback subnet exists. Systems with multiple physical adapters, VPNs, or
container networks must select the intended interface explicitly.

A missing, expired, incorrectly signed, or machine/subnet-mismatched license
fails closed with a clear operator error. There is no silent full-tier fallback.

## Purchased-license activation and rehosting

After purchase, the customer receives a signed full license for their submitted
machine fingerprint and subnet.

For hardware replacement, OS reinstall that changes the identifier, NIC
replacement, or subnet relocation:

1. Customer submits the previous license ID and a new fingerprint request.
2. DRISHTI reviews and approves the rehost.
3. The issuance registry marks the old license as superseded.
4. A replacement license is issued for the new installation.

Because validation is offline, an old installation cannot be remotely disabled.
The superseded license cannot receive future updates or another rehost, and the
customer is required to remove the prior installation.

## Windows distribution

The recommended Windows package is a WiX Toolset v4 MSI with a Burn bootstrapper.
It installs:

- `backend.exe` and `worker.exe` as Windows services.
- Production frontend assets.
- A private or supported existing PostgreSQL instance.
- A controlled `qemu-img` build and required runtime files.
- Configuration, license, workspace, log, and update directories with
  least-privilege permissions.

Program files are read-only to service accounts. Mutable data lives under
`C:\ProgramData\DRISHTI\HyperShift`. The license and local configuration are
preserved by repair, upgrade, and rollback operations.

All Windows executables and installers must be Authenticode-signed and
timestamped. The current recommendation is Microsoft Azure Artifact Signing.

## Linux distribution

Docker Compose or rootless Podman remains the primary Linux distribution path.
It provides consistent PostgreSQL, `qemu-img`, frontend, backend, and worker
dependencies across distributions.

Release deployments use:

- Signed container images.
- Read-only license and configuration mounts.
- Persistent database, workspace, and license-state volumes.
- Internal-only worker networking.
- Compose or Podman Quadlet service management.

## Cumulative updates and rollback

Updates use full cumulative replacement packages initially rather than fragile
binary-difference patches. A signed manifest identifies supported starting
versions, artifacts, checksums, schema migrations, update entitlement, and
rollback compatibility.

The updater:

1. Verifies the manifest, package signature, and checksum.
2. Confirms license update entitlement and version compatibility.
3. Stages new files outside the active installation.
4. Creates a PostgreSQL backup and durable update journal.
5. Stops services safely.
6. Applies pending versioned database migrations.
7. Atomically replaces binaries and frontend assets.
8. Preserves the license, configuration, secrets, workspace, and database data.
9. Restarts services and verifies health, license, and schema status.

If an update fails before database migration, the updater restores the previous
binaries immediately. If it fails after migration, it restores the database
backup unless the migration is explicitly backward-compatible, restores the
previous files, and restarts the prior version.

## Code-protection expectations

Release Go binaries will use `-trimpath` and stripped debug/linker data. Release
frontend builds remain minified and contain no source maps or production stack
traces.

These controls increase reverse-engineering effort but do not provide perfect
secrecy. Browser JavaScript is always inspectable, and native binaries can be
disassembled or patched by a sufficiently privileged and determined user. The
backend and worker binaries are the meaningful protected surface; licensing
secrets and private signing keys are never placed in frontend code.

Go obfuscation may be evaluated in a separate qualified release job, but is not
recommended as a mandatory first-release dependency because it increases build,
testing, and support complexity.

## Key custody

Use separate keys for:

- Production customer licenses.
- Development/test licenses.
- Release manifests and update packages.
- Windows Authenticode signing.

The recommended license and manifest keys are HSM-protected ECDSA P-256 keys.
The license generator is an internal tool operated only by an authorized
DRISHTI issuer. It signs canonical license data without exposing private key
material and is excluded from every distributed product package.

## Safety boundary

Licensing is an additional authorization layer. It must never replace, weaken,
or bypass existing migration safety behavior. In particular:

- Drag-and-drop creates a draft only.
- Source VM deletion/unregistration remains prohibited.
- Trial or full licensing cannot enable an execution route locked by runtime
  mode or mutation flags.
- Runtime mode or mutation flags cannot override an invalid or restricted
  license.
- License expiration never deletes customer data.
