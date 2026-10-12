package webui

import (
	"io/fs"
	"testing"
)

func TestFSIsRootedAtBuildOutput(t *testing.T) {
	if _, err := fs.Stat(FS(), ".gitkeep"); err != nil {
		t.Fatalf("dist placeholder not embedded at root: %v", err)
	}
	if Built() {
		if _, err := fs.Stat(FS(), "assets"); err != nil {
			t.Fatalf("built UI without assets: %v", err)
		}
	}
}
