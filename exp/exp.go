package exp

import "time"

// Dur is used to UnmarshalTOML into a time.Duration value.
type Dur struct{ value time.Duration }

// UnmarshalTOML parses a duration type from a config file.
func (v *Dur) UnmarshalTOML(data []byte) error {
	unquoted := string(data[1 : len(data)-1])
	dur, err := time.ParseDuration(unquoted)
	if err == nil {
		v.value = dur
	}
	return err
}

// UnmarshalJSON parses a duration type from a config file.
func (v *Dur) UnmarshalJSON(data []byte) error {
	return v.UnmarshalTOML(data)
}

// UnmarshalText parses a duration type from a config file.
func (v *Dur) UnmarshalText(data []byte) error {
	return v.UnmarshalTOML(data)
}

// Set a duration to a Dur type.
func (v *Dur) Set(val time.Duration) {
	v.value = val
}

// Add time to a duration.
func (v *Dur) Add(val time.Duration) {
	v.value += val
}

// Value of a Dur type.
func (v *Dur) Value() time.Duration {
	return v.value
}

// String representation of a duration.
func (v *Dur) String() string {
	return v.value.Round(time.Second).String()
}
