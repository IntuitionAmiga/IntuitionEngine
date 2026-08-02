#ifndef IE64_ASSERT_H
#define IE64_ASSERT_H

#include <ie64.h>

#ifdef NDEBUG
#define assert(expression) ((void)0)
#else
#define assert(expression) ((expression) ? (void)0 : __ie64_assert_fail())
#endif

#endif
