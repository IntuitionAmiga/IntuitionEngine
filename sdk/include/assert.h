#ifndef IE64_ASSERT_H
#define IE64_ASSERT_H

_Noreturn void __builtin_ie64_halt(void);

#ifdef NDEBUG
#define assert(expression) ((void)0)
#else
#define assert(expression) ((expression) ? (void)0 : __builtin_ie64_halt())
#endif

#endif
