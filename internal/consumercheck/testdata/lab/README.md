# Frozen Lab CNF inputs

These synthetic formulas were emitted by the original Pouch Lab H2/H6 encoders
in Calx conformance run `consumers-20261018-02`. That run used Python 3.14.8 and
verified its H6 SAT trace with the independent Lab reference. Its complete
ignored historical artifacts remain in the originating checkout.

The maintained Go tool uses only the exported DIMACS formulas, their SHA256s,
expected SAT/UNSAT statuses and the original Lab source hashes in manifest.json.
It neither includes nor executes the Python sources. H2 contributes eight guard
queries; H6 contributes exact horizons zero through four. All thirteen new solver
answers must satisfy the normal Calx assignment/proof requirements.

Source-hash matching binds these fixtures to that original source snapshot. It
does not constitute a fresh run of the encoder or semantic reference. Changes
require a separately identified fixture revision, not overwriting this history.
