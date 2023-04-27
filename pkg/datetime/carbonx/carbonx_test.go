package carbonx

import (
	fmt "PowerX/pkg/printx"
	"testing"
	"time"
)

func TestBuildTree(t *testing.T) {

	time.FixedZone(time.UTC.String(), 0)
	//lc, _ := time.LoadLocation(time.UTC.String())
	//fmt2.Dump(time.UTC.String(), lc)
	now := time.Now().UTC()
	tUTC := now.UTC()
	fmt.Dump(now, tUTC)

}
