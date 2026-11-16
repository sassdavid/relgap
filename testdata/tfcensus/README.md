# tfcensus inventory fixtures

`tf-fleet.json` is a byte-for-byte copy of tfcensus `testdata/golden/inventory/tf-fleet.json` at
commit 80df9ff (2026-11-16), the output of `tfcensus inventory tf-fleet --format json`. It
validates against tfcensus `schemas/inventory-v1.json`, which hasn't changed since tfcensus
v0.1.0. Copy it again when that golden changes, and don't edit it here: scripts that need another
shape derive it in their own archive.
