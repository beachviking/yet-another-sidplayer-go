package resid

// Model selects the SID chip: 6581 or 8580
type Model byte
type SamplingMethod byte

type reg4 uint8
type reg8 uint8
type reg12 uint16
type reg16 uint16
type reg24 uint32
type CycleCount int
type sound_sample int

const (
	// 6581 SID
	MOS6581 Model = iota

	// 8580 SID
	MOS8580
)

const (
	// Fast
	SAMPLE_FAST SamplingMethod = iota

	// Interpolate
	SAMPLE_INTERPOLATE
)