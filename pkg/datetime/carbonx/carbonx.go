package carbonx

import "github.com/golang-module/carbon/v2"

func GetWeekDaysFromDay(currentDay *carbon.Carbon, formatDate func(formatD *carbon.Carbon) *carbon.Carbon) (*carbon.Carbon, *carbon.Carbon) {

	startDate := currentDay.StartOfWeek()
	endDate := currentDay.EndOfWeek()

	if formatDate != nil {
		startDate = *formatDate(&startDate)
		endDate = *formatDate(&endDate)
	}

	return &startDate, &endDate
}
