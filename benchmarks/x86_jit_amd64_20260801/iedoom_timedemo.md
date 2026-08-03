# IEDoom DEMO1 acceptance

Recorded on the Intel Xeon W-11955M through the native amd64 x86 JIT.

Command:

```text
make IEDOOM_TIMED_WATCHDOG='timeout 180s' x86-iedoom-timedemo
```

The target used `../chocolate-doom/build/iedoom_timedemo.ie86`, the `DEMO1`
lump from `../chocolate-doom/DOOM1.WAD`, `IE_NO_IPC=1`, and the checked-in
`bench/measure_timedemo.ies` observer. It emitted:

```text
[bench] gametic=350 target=350 window_ms=23026 boot_ms=909
[bench] IEDOOM_TIMEDEMO_COMPLETE gametic=350 target=350
```

The observer's generated result was `mode=jit window_ms=23026 boot_ms=909
tics=349`. This is a completion and timing record, not a cross-host speedup
claim.
