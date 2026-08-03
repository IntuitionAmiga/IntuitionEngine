# IE64 C toolchain V2

The package targets bare-metal `ie64-unknown-none` and runs on Linux x86-64
with glibc 2.38 or newer.
Its `bin` directory contains `ie64-cproc`, `cproc-qbe`, `qbe`, `ie64asm`,
`ie64dis`, `ie64ld`, `ie64-ar` and `ie64-ranlib`. Target libraries are under
`lib/ie64-unknown-none`, headers are under `include`, and `include/ie64.inc`
contains assembly convenience definitions.

Extract the archive and add its tools to `PATH`:

```sh
tar -xf ie64-toolchain-v2-linux-amd64.tar.xz
export PATH="$PWD/ie64-toolchain-v2-linux-amd64/bin:$PATH"
ie64-cproc -o hello.ie64 hello.c
ie64-cproc -o program.ie64 main.c worker.c
```

Assemble a source, build a static library, and inspect an image:

```sh
ie64asm -I "$PWD/ie64-toolchain-v2-linux-amd64/include" -c -o start.o start.s
ie64-ar rcs libwork.a worker.o
ie64-ranlib libwork.a
ie64-cproc -o program.ie64 main.o libwork.a
ie64dis program.ie64
```

Select a relocated or alternate sysroot with
`ie64-cproc --sysroot /path/to/toolchain`. See `IE64_ABI_V2.md`,
`IE64_ISA.md`, and `architecture.md` in `share/ie64/docs` for the ABI and
machine contracts.
