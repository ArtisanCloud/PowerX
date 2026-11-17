package hostmonitor

import "testing"

func TestMountFixture(t *testing.T) {
	m := parseMounts("1 0 8:1 / / rw - ext4 /dev/root rw\n2 1 0:1 / /proc rw - proc proc rw\n3 1 0:2 / /remote\\040share rw - nfs server:/x rw\n4 1 0:3 / / rw - ext4 /dev/root rw\n")
	if len(m) != 2 || m[1][0] != "/remote share" || m[1][1] != "nfs" {
		t.Fatal(m)
	}
}
