# Writing comments for SDK examples

Write comments for the IE guest machine. Name the guest CPU, RAM, MMIO device
or guest file involved, rather than the Go implementation that provides it.
Open with the example's purpose and, when useful, its target, assets, and a
`go run .` launch command. Disk-backed examples must include `-file-root .`.

Organise longer examples around real phases: data layout, initialisation,
per-frame state, submission or hot loop, presentation, and cleanup. Explain
why a choice matters for correctness, representation, portability, or the
hardware. Explain an instruction only when its purpose is otherwise unclear.

Use natural British English. Avoid all-caps `WHY` headings, marketing,
historical padding, generic tutorial slogans, stale commands, em dashes, and
unmeasured performance claims. The Host SDK is a cross-development toolchain,
not part of the guest runtime. Keep shared ports aligned and state only their
real CPU, OS, API, or layout differences.
