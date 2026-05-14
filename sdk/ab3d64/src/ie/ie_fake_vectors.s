	include "ie_system.i"

org FAKE_LIB_BASE+_LVOWaitTOF
ie_fake_waittof_vector:
	bra	ie_fake_lvo_rts

org FAKE_LIB_BASE+_LVOCacheControl
ie_fake_cachecontrol_vector:
	bra	ie_fake_lvo_rts

	org FAKE_LIB_BASE+$100
ie_fake_lvo_rts:
	rts
