package diagnostics

import ticketbridge "github.com/ArtisanCloud/PowerX/internal/service/integration/ticketbridge"

// Options describe diagnostics runtime dependencies.
type Options struct {
	Template        *ReportTemplate
	Masker          *Masker
	TicketBridge    ticketbridge.Service
	FallbackLogBase string
}

func (o Options) fallbackURL(reportID string) string {
	base := o.FallbackLogBase
	if base == "" || reportID == "" {
		return ""
	}
	base = trimTrailingSlash(base)
	return base + "/" + reportID
}

func trimTrailingSlash(input string) string {
	if input == "" {
		return input
	}
	for len(input) > 0 && input[len(input)-1] == '/' {
		input = input[:len(input)-1]
	}
	return input
}
