package runner

import (
	"testing"
	"time"
)

func TestParseDailyScheduleAndNext(t *testing.T) {
	schedule, err := ParseDailySchedule("18:00, 06:00,00:00,12:00,06:00")
	if err != nil {
		t.Fatal(err)
	}
	location := time.FixedZone("test", 3*60*60)
	after := time.Date(2026, 9, 16, 5, 30, 0, 0, location)
	if got := schedule.Next(after, location); !got.Equal(time.Date(2026, 9, 16, 6, 0, 0, 0, location)) {
		t.Fatalf("next=%v", got)
	}
	after = time.Date(2026, 9, 16, 18, 0, 1, 0, location)
	if got := schedule.Next(after, location); !got.Equal(time.Date(2026, 9, 17, 0, 0, 0, 0, location)) {
		t.Fatalf("next day=%v", got)
	}
}

func TestParseDailyScheduleRejectsInvalidValue(t *testing.T) {
	for _, value := range []string{"", "24:00", "12:60", "noon"} {
		if _, err := ParseDailySchedule(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}
