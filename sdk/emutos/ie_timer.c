#include "emutos.h"
#include "ie_machine.h"
#include "vectors.h"

/*
 * Install IE timer infrastructure.
 *
 * Must be called after vecs_init() so the default exception vectors are
 * already in place.
 *
 * Interrupt enable remains in the normal BIOS flow (set_sr() in bios_init()).
 */
void ie_timer_install(void)
{
    VEC_LEVEL5 = int_timerc;

#if !CONF_WITH_MFP
    /* Keep the _5MS cookie target in sync with the active timer handler. */
    vector_5ms = int_timerc;
#endif
}

void ie_timer_handle_l5(void)
{
    /* Timer work runs in int_timerc from vectors.S. */
}

void ie_timer_handle_l4(void)
{
    /* VBL work is handled by _int_vbl in vectors.S */
}
