# Bounded consumer conformance

The consumers remain unchanged. `scripts/check_consumers.py` requires Python
3.10+ (validated with 3.14), explicit source roots and a new output directory.
It snapshots selected source files, records their SHA256s, and builds external
Go modules with a local replace pointing at this Calx checkout. Those temporary
replaces and copies are confined to the ignored output.

```sh
python3.14 scripts/check_consumers.py \
  --core "$AHE_CORE_ROOT" --mcp "$AHE_MCP_ROOT" \
  --pouch "$POUCH_PRODUCT_ROOT" --lab "$POUCH_LAB_ROOT" \
  --out artifacts/consumer-check-NEW
```

Set CALX_CADICAL, CALX_CADICAL_SHA256, CALX_DRAT and CALX_DRAT_SHA256 first.
The script builds its own CLI binary from the current source.

| Boundary | What runs | Excluded scope |
| --- | --- | --- |
| AHE Core and MCP | Actual compileConsistency and consistencyText function bodies, source condition types, minimal in-memory scope stubs; compatible/conflicting formulas and incomplete-input rejection; real checked solver | DB scope reader, currentness, RLS, persistence, admission, deployed MCP |
| Pouch Product | Source planning/validation/projection in an external module; exact DIMACS comparison for horizons 0–3; native Search through a Calx adapter; forward and return original-rule replay; topology matrices against the original projection | Full CLI, deployment, governance/network calls, real repair/execution |
| Pouch Lab | Source H2 guard queries (8), H6 signed/copy encoder at horizons 0–4 (5), Python→CLI checked solve; SAT trace decoded by Lab and replayed by its separate original reference; typed input rejected by finite v1 | All Lab profiles, historical-proof import, H19 typed/copy lowering into finite v1, new research or model scoring |

The H6 path remains Lab encoder → DIMACS → Calx SAT, not a claim that finite v1
supports copy effects. Independent receivers are not replaced by Calx.
The standalone solve-finite example is also exercised through the CLI.

Source attribution and current checkpoints are recorded with each run. Source
snapshots and output byte hashes are conformance evidence for that snapshot;
passing this harness is not native adoption or release authorization.
