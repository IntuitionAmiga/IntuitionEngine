# Picolibc source and licence audit

The IE64 sysroot uses Picolibc 1.8.12 from the sibling repository's `ie`
branch. The release build accepts only the full revision recorded in
`ie64-v3-release-inputs.conf`, requires a clean Git checkout, and packages
`COPYING.picolibc` unchanged.

The baseline is upstream revision
`2ae376c6cdf4fef90ca2388ecf7a07457fa63cff`, reached by the initial IE branch
revision `c24327022100f4478a15b220177affb84af5f3ba`. Picolibc combines BSD-style
and other permissive source licences recorded by its file headers and licence
notice. The build does not import GPL runtime objects.

IE64-specific code is confined to `libc/machine/ie64`, the small generic stdio
ownership and exit-cleanup changes required by that port, and the target
configuration and build lists. New `<stdbit.h>`, `<stdckdint.h>` and sized
deallocation sources carry `SPDX-License-Identifier: BSD-3-Clause`. Existing
modified Picolibc files retain their original licence headers. IntuitionEngine,
QBE and cproc licence texts are packaged separately beside
`COPYING.picolibc`.

The package contains installed headers, target archives, host tools,
documentation and licence notices only. It excludes source checkouts,
intermediate objects, generated build directories and ignored local output.
