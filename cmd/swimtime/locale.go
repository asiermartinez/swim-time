package main

// locale holds what differs between the language versions of the report:
// the prose lives in one README template per language, the rest here.
type locale struct {
	Code           string // charts and calendars go in a directory with this name
	ReadmePath     string // relative to the repository root
	Readme         string // name of the README template
	CalendarReadme string // name of the template for calendar/<Code>/README.md

	// Chart text. Title and the no-data messages also serve as the charts'
	// accessible names, so they are full phrases.
	Days         [7]string
	Title        string // takes the pool name
	SummaryTitle string
	Max          string // top of the summary's legend
	NoData       string // summary header without data
	NoDataWeeks  string // pool chart without data
	NoDataRow    string // summary row without data

	// Calendar text.
	WeekDays   [7]string
	CalName    string // takes the pool name
	CalDesc    string // takes the pool name
	EventTitle string // takes the rank
	EventDesc  string // takes the pool name, the weekday and the people
	EventAforo string // takes the capacity
	EventRank  string // takes the rank
}

// locales lists every version `swimtime render` writes.
var locales = []locale{en, es}

var en = locale{
	Code:           "en",
	ReadmePath:     "README.md",
	Readme:         "README.en.md.tmpl",
	CalendarReadme: "calendar.en.md.tmpl",
	Days:           [7]string{"MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"},
	Title:          "Typical occupancy of %s",
	SummaryTitle:   "Typical occupancy of the indoor municipal pools",
	Max:            "max.",
	NoData:         "No data",
	NoDataWeeks:    "No data in recent weeks",
	NoDataRow:      "no data",

	WeekDays:   [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"},
	CalName:    "%s pool",
	CalDesc:    "Quietest hours of %s, based on the typical occupancy of each day of the week.",
	EventTitle: "Pool (Quiet hour #%d)",
	EventDesc:  "%s: on a typical %s there are about %.0f people at this time",
	EventAforo: " (capacity %d)",
	EventRank:  ". It is the #%d quietest hour of the day.",
}

var es = locale{
	Code:           "es",
	ReadmePath:     "es/README.md",
	Readme:         "README.es.md.tmpl",
	CalendarReadme: "calendar.es.md.tmpl",
	Days:           [7]string{"LUN", "MAR", "MIÉ", "JUE", "VIE", "SÁB", "DOM"},
	Title:          "Ocupación típica de %s",
	SummaryTitle:   "Ocupación típica de las piscinas municipales cubiertas",
	Max:            "máx.",
	NoData:         "Sin datos",
	NoDataWeeks:    "Sin datos en las últimas semanas",
	NoDataRow:      "sin datos",

	WeekDays:   [7]string{"lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"},
	CalName:    "Piscina %s",
	CalDesc:    "Horas más tranquilas de %s, según la ocupación típica de cada día de la semana.",
	EventTitle: "Piscina (Hora tranquila #%d)",
	EventDesc:  "%s: un %s típico hay unas %.0f personas en esta hora",
	EventAforo: " (aforo %d)",
	EventRank:  ". Es la %dª hora más tranquila del día.",
}
