package timeutil

import (
	"fmt"
	"time"
)

const jakartaTimezone = "Asia/Jakarta"

func JakartaLocation() *time.Location {
	loc, err := time.LoadLocation(jakartaTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func NowJakarta() time.Time {
	return time.Now().In(JakartaLocation())
}

func TodayJakarta() string {
	return NowJakarta().Format("2006-01-02")
}

func FormatDateIndonesian(t time.Time) string {
	months := []string{
		"Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	local := t.In(JakartaLocation())
	return fmt.Sprintf("%d %s %d", local.Day(), months[local.Month()-1], local.Year())
}
