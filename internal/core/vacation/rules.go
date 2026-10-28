package vacation

import (
	"errors"
	"time"
)

var ErrInvalidDateRange = errors.New("invalid vacation date range")

// CountDays считает календарные дни отпуска включительно.
func CountDays(start, end time.Time) (int, error) {
	// Убираем время и переводим даты в UTC.
	normalize := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}

	start = normalize(start)
	end = normalize(end)

	if end.Before(start) {
		return 0, ErrInvalidDateRange
	}

	return int(end.Sub(start).Hours()/24) + 1, nil
}

// HasEnoughBalance проверяет, хватает ли дней для отпуска.
func HasEnoughBalance(balance, requestedDays int) bool {
	if balance < 0 || requestedDays <= 0 {
		return false
	}

	return balance >= requestedDays
}
