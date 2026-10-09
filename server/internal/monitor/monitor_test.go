package monitor

import "testing"

func TestCameraIDFromPath(t *testing.T) {
	cases := map[string]int64{"cam1": 1, "cam42_sub": 42, "cam": 0, "camera": 0, "other": 0, "cam7x": 0}
	for path, want := range cases {
		if got := cameraIDFromPath(path); got != want {
			t.Errorf("%s: got %d, want %d", path, got, want)
		}
	}
}
