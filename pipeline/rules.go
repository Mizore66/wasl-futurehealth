package main

// v6: programme rule table. The same linker + closure engine runs every screening programme; only this table differs.
// Sources (DoH Abu Dhabi standards, sources/doh-standards/):
//   CRC      crc.txt 5.5.4.2  "positive [FIT] result must be scheduled for follow-up colonoscopy within 31 days of referral"
//            crc.txt 5.5.2    results to participant < 15 working days  (modelled as 21 calendar days from referral, as v5)
//            crc.txt App. 2   KPI: referral -> colonoscopy within 31 days, acceptable >90%, desirable >95%
//   Cervical cervical.txt 10.6.2  written result to the woman within 21 days of the date the specimen was taken
//            cervical.txt App. 5  colposcopy appointment: HSIL or greater urgent 1-2 weeks; LSIL soon 2-4 weeks;
//                                 ASC-US / ASC-H routine 4-8 weeks  (we use the upper bound: 14 / 28 / 56 days)
//            cervical.txt App. 3  negative cytology + HPV positive -> colposcopy (App. 5 gives no timeframe: routine 56 d assumed)
//            cervical.txt App. 4  KPI "Time to Colposcopy": HSIL+/ASC-H with colposcopy within 3/6/9/12 months of the index
//                                 test, acceptable 80%, desirable 88% (our 120-day window measures the 3-month point)
import "strings"

type ProgRule struct {
	ID        string
	Proc      string  // procedure that closes the loop (Appointment.serviceType text)
	NotifDays float64 // result-notification SLA
	NotifFrom string  // "referral" | "specimen"
	KPIDays   float64 // programme KPI window for "procedure done"
	Tpl       string  // approved message template family
}

var Rules = map[string]ProgRule{
	"crc":      {"crc", "Colonoscopy", 21, "referral", 31, "fitfu"},
	"cervical": {"cervical", "Colposcopy", 21, "specimen", 90, "ccsfu"},
}

// DeadlineDays: days from referral by which the closing procedure must happen.
func DeadlineDays(prog, priority string) float64 {
	if prog == "cervical" {
		switch priority {
		case "urgent":
			return 14
		case "soon":
			return 28
		}
		return 56
	}
	return 31
}

// CervPriority maps the index cytology/HPV category to the App. 5 priority.
func CervPriority(cat string) string {
	switch cat {
	case "HSIL+":
		return "urgent"
	case "LSIL":
		return "soon"
	}
	return "routine" // ASC-US, ASC-H, HPV+/NILM
}

// KPI cohort for cervical "Time to Colposcopy": HSIL+/ASC-H only.
func InKPICohort(prog, cat string) bool {
	if prog == "cervical" {
		return cat == "HSIL+" || cat == "ASC-H"
	}
	return true
}

func progOfText(s string) string {
	s = strings.ToLower(s)
	if strings.Contains(s, "colposcop") || strings.Contains(s, "cervical") {
		return "cervical"
	}
	return "crc"
}
