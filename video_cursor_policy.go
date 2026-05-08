package main

func shouldHideSystemCursor(fullscreen, profileHidden bool) bool {
	return fullscreen || profileHidden
}
