package carbonx

import (
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
)

var DefaultTimeZone = carbon.UTC

const DateFormat = "Y-m-d"
const TimeFormat = "H:i:s"
const DatetimeFormat = DateFormat + " " + TimeFormat
const GoDatetimeFormat = "2006-01-02 15:04:05"
const GoDateFormat = "2006-01-02"

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
func GetCurrentDaysFromDay(currentDay *carbon.Carbon, formatDate func(formatD *carbon.Carbon) *carbon.Carbon) (*carbon.Carbon, *carbon.Carbon) {

	startDate := currentDay.StartOfDay()
	endDate := currentDay.EndOfDay()

	if formatDate != nil {
		startDate = *formatDate(&startDate)
		endDate = *formatDate(&endDate)
	}

	return &startDate, &endDate
}

func ConvertDateStringToDatetime(strDate string) (*carbon.Carbon, error) {
	now := carbon.Now()
	var cDate carbon.Carbon
	if strDate != "" {
		cDate = carbon.Parse(strDate)
		if cDate.IsInvalid() {
			return nil, errors.New("查询的当前时间格式无效")
		}
	} else {
		cDate = now
	}

	return &cDate, nil
}
