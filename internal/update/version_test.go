package update

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		local, remote string
		want          bool
	}{
		{"0.1.9", "0.2.0", true},
		{"0.1.9", "v0.2.0", true},
		{"0.2.0", "0.1.9", false},
		{"0.1.9", "0.1.9", false},
		{"v0.1.9", "0.1.9", false},
		{"0.0.0", "1.0.0", false},
		{"v0.0.0", "0.1.0", false},
		{"garbage", "1.0.0", false},
		{"1.0.0", "not-a-version", false},
		{"1.0.0", "1.0.1-beta", true},
		{"1.2.3", "1.10.0", true},
	}
	for _, tc := range cases {
		if got := IsNewer(tc.local, tc.remote); got != tc.want {
			t.Fatalf("IsNewer(%q, %q)=%v want %v", tc.local, tc.remote, got, tc.want)
		}
	}
}

func TestCompare(t *testing.T) {
	cmp, err := Compare("1.2.3", "1.2.4")
	if err != nil || cmp != -1 {
		t.Fatalf("Compare 1.2.3 vs 1.2.4 = %d %v", cmp, err)
	}
	cmp, err = Compare("2.0.0", "1.9.9")
	if err != nil || cmp != 1 {
		t.Fatalf("Compare 2.0.0 vs 1.9.9 = %d %v", cmp, err)
	}
}
