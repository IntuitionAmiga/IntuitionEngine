static int private_object = 11;
static const char private_text[] = "unit-a";
static double private_fp(void) { return 1.25; }
static int private_function(void) { return private_object + private_text[5]; }

struct cross_pair { long integer; double real; };

struct cross_pair
cross_make_pair(long integer, double real)
{
	struct cross_pair result = {integer, real};
	return result;
}

long
cross_unit_a(void)
{
	return private_function() + (private_fp() == 1.25 ? 100 : 0);
}
