# fastid
## Purpose
Generate distributed 64-bit identifiers with configurable bit allocation for time, machine, and sequence.

## Use When
You need monotonic IDs with machine/time segmentation; use `ConstructConfig` plus `GenInt64ID`.

## Avoid When
A centralized ID service already exists or you require UUID/v4 semantics.

## Key Entry Points
- `ConstructConfig`, `ConstructConfigWithMachineID`
- `GenInt64ID`, `InitWithMachineID`
- `GetTimeMillFromFastID`, `GetMachineIDFromFastID`, `MinFastIdAt`

## Notes
`GenInt64ID` relies on the global config; if you set `InitWithMachineID`, keep the machine bits aligned with the DSN.

## Business Usage
- Business code uses `GenInt64ID()` for account gamer IDs, combat IDs, mail IDs, and equipment instance IDs, so these IDs are assumed sortable and cheap to generate locally.
- Public mail logic also uses `MinFastIdAt(lastCheckTime)` to scan only newer global mails. Do not replace these IDs with opaque random IDs if time-window queries still depend on ordering.
- Some call sites further transform generated IDs, for example one equipment path divides by `1000`; agents should not assume every persisted fastid keeps identical granularity.
