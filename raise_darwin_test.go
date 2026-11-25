//go:build darwin

package main

import "testing"

func TestPinWindowLevel(t *testing.T) {
	if pinWindowLevel <= nsStatusWindowLevel {
		t.Errorf("pinWindowLevel = %d, want > %d (menu bar)", pinWindowLevel, nsStatusWindowLevel)
	}
	if pinWindowLevel > nsPopUpMenuWindowLevel {
		t.Errorf("pinWindowLevel = %d, want <= %d (NSMenu context menus)", pinWindowLevel, nsPopUpMenuWindowLevel)
	}
	if pinWindowLevel >= nsScreenSaverWindowLevel {
		t.Errorf("pinWindowLevel = %d, want < %d (capture overlay)", pinWindowLevel, nsScreenSaverWindowLevel)
	}
	if nsFloatingWindowLevel >= nsModalPanelWindowLevel {
		t.Errorf("nsFloatingWindowLevel = %d, want < %d (modal save panel)", nsFloatingWindowLevel, nsModalPanelWindowLevel)
	}
}
