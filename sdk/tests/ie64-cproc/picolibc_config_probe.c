#include <picolibc.h>

#ifndef __INIT_FINI_ARRAY
#error "Picolibc IE64 requires init/fini arrays"
#endif
#ifndef __STDIO_EXIT_FLUSH
#error "Picolibc IE64 requires stdio exit flushing"
#endif

#include "local-onexit.h"

#ifndef ENABLE_PICOLIBC_EXIT
#error "Picolibc IE64 requires exit handler support"
#endif

int picolibc_config_probe;
