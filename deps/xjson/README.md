# xjson
## Purpose
Dynamic JSON wrapper for path-based read/write/delete plus JSON load/dump helpers.

## Use When
You need to traverse or mutate loosely typed JSON without defining structs upfront.

## Avoid When
You already have stable typed models; plain `encoding/json` into structs is simpler and safer.

## Key Entry Points
- `Json`
- `TryLoad`, `Load`
- `Json.Get`, `GetJson`, `Set`, `Delete`
- `LoadFileTo`, `LoadBytesTo`, `LoadStringTo`
- `TryDump`, `Dump`

## Notes
Path operations accept map keys and slice indexes; special constants like `APPEND` and `INDEX_LAST` control array mutations.

## Business Usage
- Business-layer use is currently limited and intentional: `robot/controller/web.go` uses `LoadStringTo` to bind external web/debug parameters into request structs.
- Agents should not infer that gameplay handlers prefer dynamic JSON traversal. The normal business path is still proto-first; `xjson` is a side entry for flexible debug/web input.
- Because this usage sits near external input, prefer typed target structs over free-form `Json` mutation unless the caller already needs dynamic shape handling.
