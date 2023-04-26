package carbonx

import "github.com/golang-module/carbon/v2"

func GetWeekDaysFromDay(currentDay *carbon.Carbon, formatDate func(formatD *carbon.Carbon) *carbon.Carbon) (startDate *carbon.Carbon, endDate *carbon.Carbon) {

	*startDate = currentDay.StartOfWeek()
	*endDate = currentDay.StartOfWeek()

	if formatDate != nil {
		startDate = formatDate(startDate)
		endDate = formatDate(endDate)
	}

	return startDate, endDate
}
