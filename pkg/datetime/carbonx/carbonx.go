package carbonx

import "github.com/golang-module/carbon/v2"

var DefaultTimeZone = carbon.UTC

const DateFormat = "Y-m-d"
const TimeFormat = "H:i:s"
const DatetimeFormat = DateFormat + " " + TimeFormat

type CarbonDatetime struct {
	C *carbon.Carbon
}

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

func GetWeekDaysFromDay(currentDay *carbon.Carbon, formatDate func(formatD *carbon.Carbon) *carbon.Carbon) (*carbon.Carbon, *carbon.Carbon) {

	startDate := currentDay.StartOfWeek()
	endDate := currentDay.EndOfWeek()

	if formatDate != nil {
		startDate = *formatDate(&startDate)
		endDate = *formatDate(&endDate)
	}

	return &startDate, &endDate
}
