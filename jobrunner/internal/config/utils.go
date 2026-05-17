package config

import (
	"fmt"
	"time"
)

func deadlineValueToString(value interface{}) string {
	switch v := value.(type) {
	case time.Time:
		return v.Format(time.RFC3339)
	case time.Duration:
		return v.String()
	case DeadlineValue:
		if v.Time != nil {
			return v.Time.Format(time.RFC3339)
		}
		if v.Duration != nil {
			return v.Duration.String()
		}
		return "empty deadline value"
	default:
		return fmt.Sprintf("%v", v)
	}
}
