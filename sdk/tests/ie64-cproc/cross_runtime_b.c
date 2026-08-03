static int private_object = 22;
static const char private_text[] = "unit-b";
static double private_fp(void) { return 2.5; }
static int private_function(void) { return private_object + private_text[5]; }

struct cross_pair { long integer; double real; };

long
cross_read_pair(struct cross_pair value)
{
	value.integer += value.real == 4.5 ? 10 : 1000;
	return value.integer;
}

long
cross_unit_b(void)
{
	return private_function() + (private_fp() == 2.5 ? 200 : 0);
}
