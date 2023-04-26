package carbonx

import (
	"github.com/golang-module/carbon/v2"
)

type CarbonDatetime struct {
	C *carbon.Carbon
}

var DefaultTimeZone = carbon.UTC

const DATE_FORMAT = "Y-m-d"
const TIME_FORMAT = "H:i:s"
const DATETIME_FORMAT = DATE_FORMAT + " " + TIME_FORMAT

func CreateCarbonDatetime(c carbon.Carbon) (dt *CarbonDatetime) {

	dt = &CarbonDatetime{
		&c,
	}
	return dt
}

func (dt *CarbonDatetime) SetDatetime(c carbon.Carbon) {
	dt.C = &c
}

func (dt *CarbonDatetime) SetTimezone(timezone string) *CarbonDatetime {
	dt.C.SetTimezone(timezone)
	dt.C.AddHours(8)

	return dt
}

func GetCarbonNow() carbon.Carbon {
	now := carbon.Now()

	now = now.SetTimezone(DefaultTimeZone)

	return now
}
