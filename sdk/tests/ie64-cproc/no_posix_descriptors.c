#include <stdio.h>

void *must_not_compile = (void *)&fdopen;
int (*must_not_compile_either)(FILE *) = fileno;
