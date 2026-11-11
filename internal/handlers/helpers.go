package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// paramID parses a positive integer path parameter (:id).
func paramID(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// parseFlexibleDate parses a date/timestamp sent by clients. Accepts
// "2006-01-02" and RFC3339 shapes; an empty string yields (nil, true).
func parseFlexibleDate(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, true
		}
	}
	return nil, false
}
