package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration preserves whether a duration was omitted. An explicit "0s" is
// therefore distinguishable from the zero-value default used by omitempty.
type Duration string

func (d Duration) Value() time.Duration {
	if d == "" {
		return 0
	}
	value, _ := time.ParseDuration(string(d))
	return value
}

func (d Duration) IsSet() bool { return d != "" }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(d))
}

func (d *Duration) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("duration must be a string: %w", err)
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	*d = Duration(value.String())
	return nil
}
