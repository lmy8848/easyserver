package cron

import "testing"

func TestBuildOnCalendar(t *testing.T) {
	cases := []struct {
		name string
		form ScheduleForm
		want string
	}{
		{"minutely", ScheduleForm{Frequency: "minutely", EveryN: 5}, "*:00/5"},
		{"minutely-1", ScheduleForm{Frequency: "minutely", EveryN: 1}, "*:00/1"},
		{"hourly", ScheduleForm{Frequency: "hourly", EveryN: 3}, "*-*-* 0/3:00:00"},
		{"daily", ScheduleForm{Frequency: "daily", Time: "03:30"}, "*-*-* 03:30:00"},
		{"daily-single-digits", ScheduleForm{Frequency: "daily", Time: "3:5"}, "*-*-* 03:05:00"},
		{"weekly-one-day", ScheduleForm{Frequency: "weekly", Time: "09:00", Weekdays: []string{"Mon"}}, "Mon *-*-* 09:00:00"},
		{"weekly-multi", ScheduleForm{Frequency: "weekly", Time: "09:00", Weekdays: []string{"Mon", "Wed", "Fri"}}, "Mon,Wed,Fri *-*-* 09:00:00"},
		{"weekly-dedup", ScheduleForm{Frequency: "weekly", Time: "09:00", Weekdays: []string{"Mon", "Mon"}}, "Mon *-*-* 09:00:00"},
		{"monthly", ScheduleForm{Frequency: "monthly", Time: "23:59", DayOfMonth: 1}, "*-*-01 23:59:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := BuildOnCalendar(c.form)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("want %q, got %q", c.want, got)
			}
		})
	}
}

func TestBuildOnCalendar_Invalid(t *testing.T) {
	cases := []struct {
		name string
		form ScheduleForm
	}{
		{"minutely-zero", ScheduleForm{Frequency: "minutely", EveryN: 0}},
		{"hourly-zero", ScheduleForm{Frequency: "hourly", EveryN: 0}},
		{"daily-bad-time", ScheduleForm{Frequency: "daily", Time: "25:00"}},
		{"daily-min", ScheduleForm{Frequency: "daily", Time: "03:60"}},
		{"weekly-no-days", ScheduleForm{Frequency: "weekly", Time: "09:00"}},
		{"weekly-bad-day", ScheduleForm{Frequency: "weekly", Time: "09:00", Weekdays: []string{"Funday"}}},
		{"monthly-day-zero", ScheduleForm{Frequency: "monthly", Time: "09:00", DayOfMonth: 0}},
		{"monthly-day-32", ScheduleForm{Frequency: "monthly", Time: "09:00", DayOfMonth: 32}},
		{"unknown-freq", ScheduleForm{Frequency: "yearly"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := BuildOnCalendar(c.form); err == nil {
				t.Fatalf("expected error for %v, got nil", c.form)
			}
		})
	}
}

func TestDescribeSchedule(t *testing.T) {
	got := DescribeSchedule(ScheduleForm{Frequency: "daily", Time: "03:30"})
	if got != "每天 03:30 执行" {
		t.Fatalf("unexpected description: %q", got)
	}
}
