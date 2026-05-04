package main

func clipboardBoundsOK(ptr, n, cap uint32) bool {
	return n <= cap && ptr <= cap-n
}
