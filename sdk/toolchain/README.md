# IE64 C toolchain V3

This archive is a freestanding C23 compiler suite for `ie64-unknown-none`.
Use it to build flat `.ie64` programs for Intuition Engine, not hosted Linux
programs. It runs on Linux x86-64 with glibc 2.38 or newer.

The `bin` directory contains `ie64-cproc`, `cproc-qbe`, `qbe`, `ie64asm`,
`ie64dis`, `ie64ld`, `ie64-ar` and `ie64-ranlib`. Target headers are in
`include`; target libraries and `crt0.o` are in `lib/ie64-unknown-none`.

## Quick start

Extract the archive, add its tools to `PATH`, and compile a program:

```sh
tar -xf ie64-toolchain-v3-linux-amd64.tar.xz
export PATH="$PWD/ie64-toolchain-v3-linux-amd64/bin:$PATH"
ie64-cproc -o hello.ie64 hello.c
ie64-cproc -o program.ie64 main.c worker.c
```

The driver compiles C sources, assembles generated code, links the flat image,
and adds `crt0.o`, Picolibc, libm and libatomic unless disabled. Run the image
with Intuition Engine's IE64 profile. `ie64dis program.ie64` prints a useful
disassembly when diagnosing a build.

## Multiple files and libraries

Assemble a source, build a static library, and inspect an image:

```sh
ie64asm -I "$PWD/ie64-toolchain-v3-linux-amd64/include" -c -o start.o start.s
ie64-ar rcs libwork.a worker.o
ie64-ranlib libwork.a
ie64-cproc -o program.ie64 main.o libwork.a
ie64dis program.ie64
```

Input and `-l` library arguments retain the order written on the command line.
The default libraries are appended after them. Use `-nostdlib`,
`-nostartfiles`, or `-nodefaultlibs` only when supplying the corresponding
runtime pieces yourself.

## Useful options

`-E`, `-S` and `-c` stop after preprocessing, assembly, or object generation.
`-O0` through `-O3` select the documented optimisation pipelines. `-I`, `-D`,
`-U`, `-MMD`, `-MF` and `-MT` provide the usual include, macro and dependency
output controls. `-o -` writes preprocessed text or the linked image to
standard output.

Select a relocated or alternate sysroot with
`ie64-cproc --sysroot /path/to/toolchain`. See `IE64_ISA.md` and
`architecture.md` in `share/ie64/docs` for the instruction and machine
contracts.

## Scope

The suite supports the V3 flat-image ABI only. Rebuild earlier objects and
archives from source. There is no hosted POSIX environment, shared
libraries, PIE, TLS, C++ runtime or GCC/Clang compatibility promise.
