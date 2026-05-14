package main

func mapPresentationMouseToGuest(x, y, width, height int, tm *TerminalMMIO, compositor *VideoCompositor) (int, int, int, int) {
	if tm != nil {
		if nw, nh := int(tm.mouseNativeW.Load()), int(tm.mouseNativeH.Load()); nw > 0 && nh > 0 {
			if compositor != nil {
				return compositor.MapPresentationPointToNativeForSource(x, y, nw, nh)
			}
			if nw != width || nh != height {
				x = x * nw / width
				y = y * nh / height
				width = nw
				height = nh
			}
			return x, y, width, height
		}
	}
	if compositor != nil {
		return compositor.MapPresentationPointToNative(x, y)
	}
	return x, y, width, height
}
