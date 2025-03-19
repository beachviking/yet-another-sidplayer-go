package resid

// Voice represents a voice in the SID chip.
type WaveformGenerator struct {
	syncDest    *WaveformGenerator
	syncSource  *WaveformGenerator
	msbRising   bool
	accumulator reg24
	shiftreg    reg24
	freq        reg16
	pw          reg12
	waveform    reg8
	test        reg8
	ringmod     reg8
	sync        reg8

	wave__ST *[]reg8
	wave_P_T *[]reg8
	wave_PS  *[]reg8
	wave_PST *[]reg8

	model Model
}

// ----------------------------------------------------------------------------
// Constructor.
// ----------------------------------------------------------------------------
func NewWaveformGenerator() *WaveformGenerator {
	w := &WaveformGenerator{}
	w.syncSource = w
	w.SetModel(MOS6581)
	w.Reset()
	return w
}

// ----------------------------------------------------------------------------
// SID reset.
// ----------------------------------------------------------------------------
func (w *WaveformGenerator) Reset() {
	w.accumulator = 0
	w.shiftreg = 0x7ffff8
	w.freq = 0
	w.pw = 0

	w.test = 0
	w.ringmod = 0
	w.sync = 0

	w.msbRising = false
}

// ----------------------------------------------------------------------------
// Set sync source.
// ----------------------------------------------------------------------------
func (w *WaveformGenerator) SetSyncSource(source *WaveformGenerator) {
	w.syncSource = source
	source.syncDest = w
}

// ----------------------------------------------------------------------------
// Set chip model.
// ----------------------------------------------------------------------------
func (w *WaveformGenerator) SetModel(model Model) {
	w.model = model

	if w.model == MOS6581 {
		w.wave_PS = &wave6581_PS_
		w.wave_PST = &wave6581_PST
		w.wave_P_T = &wave6581_P_T
		w.wave__ST = &wave6581__ST
		return
	}

	w.wave_PS = &wave8580_PS_
	w.wave_PST = &wave8580_PST
	w.wave_P_T = &wave8580_P_T
	w.wave__ST = &wave8580__ST
}

// ----------------------------------------------------------------------------
// SID clocking - delta_t cycles.
// ----------------------------------------------------------------------------
func (w *WaveformGenerator) Clock(delta_t CycleCount) {
	// No operation if test bit is set.
	if w.test != 0 {
		return
	}

	accumulator_prev := w.accumulator

	// Calculate new accumulator value
	var delta_accumulator reg24 = reg24(delta_t) * reg24(w.freq)
	w.accumulator += delta_accumulator
	w.accumulator &= 0xffffff

	// Check whether the MSB is set high. This is used for synchronization.
	w.msbRising = (accumulator_prev&0x800000 == 0) && (w.accumulator&0x800000 != 0)

	// Shift noise register once for each time accumulator bit 19 is set high.
	// Bit 19 is set high each time 2^20 (0x100000) is added to the accumulator.
	var shift_period reg24 = 0x100000

	for delta_accumulator > 0 {
		if delta_accumulator < shift_period {
			shift_period = delta_accumulator
			// Determine whether bit 19 is set on the last period.
			// NB! Requires two's complement integer.
			if shift_period <= 0x080000 {
				// Check for flip from 0 to 1.
				if ((w.accumulator-shift_period)&0x080000 != 0) ||
					(w.accumulator&0x080000 == 0) {
					break
				}
			} else {
				// Check for flip from 0 (to 1 or via 1 to 0) or from 1 via 0 to 1.
				if ((w.accumulator-shift_period)&0x080000 != 0) &&
					(w.accumulator&0x080000 == 0) {
					break
				}
			}
		}

		// Shift the noise/random register.
		// NB! The shift is actually delayed 2 cycles, this is not modeled.
		bit0 := ((w.shiftreg >> 22) ^ (w.shiftreg >> 17)) & 0x1
		w.shiftreg <<= 1
		w.shiftreg &= 0x7fffff
		w.shiftreg |= bit0

		delta_accumulator -= shift_period
	}
}

func (w *WaveformGenerator) Synchronize() {
	// A special case occurs when a sync source is synced itself on the same
	// cycle as when its MSB is set high. In this case the destination will
	// not be synced. This has been verified by sampling OSC3.

	if w.msbRising && (w.syncDest.sync != 0) && !((w.sync != 0) && w.syncSource.msbRising) {
		w.syncDest.accumulator = 0
	}
}

// ----------------------------------------------------------------------------
// Output functions.
// NB! The output from SID 8580 is delayed one cycle compared to SID 6581,
// this is not modeled.
// ----------------------------------------------------------------------------

// No waveform:
// Zero output.
func (w *WaveformGenerator) output____() reg12 {
	return 0x000
}

// Triangle:
// The upper 12 bits of the accumulator are used.
// The MSB is used to create the falling edge of the triangle by inverting
// the lower 11 bits. The MSB is thrown away and the lower 11 bits are
// left-shifted (half the resolution, full amplitude).
// Ring modulation substitutes the MSB with MSB EOR sync_source MSB.
func (w *WaveformGenerator) output___T() reg12 {
	msb := w.accumulator

	if w.ringmod != 0 {
		msb = w.accumulator ^ w.syncSource.accumulator
	}

	msb &= 0x800000

	if msb != 0 {
		return reg12(((^w.accumulator) >> 11) & 0xffe)
	}

	return reg12((w.accumulator >> 11) & 0xffe)
}

// Sawtooth:
// The output is identical to the upper 12 bits of the accumulator.
func (w *WaveformGenerator) output__S_() reg12 { return reg12(w.accumulator >> 12) }

// Pulse:
// The upper 12 bits of the accumulator are used.
// These bits are compared to the pulse width register by a 12 bit digital
// comparator; output is either all one or all zero bits.
// NB! The output is actually delayed one cycle after the compare.
// This is not modeled.
//
// The test bit, when set to one, holds the pulse waveform output at 0xfff
// regardless of the pulse width setting.
//

func (w *WaveformGenerator) output_P__() reg12 {
	if (w.test != 0) || ((w.accumulator >> 12) >= reg24(w.pw)) {
		return 0xfff
	}

	return 0x000
}

// Noise:
// The noise output is taken from intermediate bits of a 23-bit shift register
// which is clocked by bit 19 of the accumulator.
// NB! The output is actually delayed 2 cycles after bit 19 is set high.
// This is not modeled.
//
// Operation: Calculate EOR result, shift register, set bit 0 = result.
//
//                        ----------------------->---------------------
//                        |                                            |
//                   ----EOR----                                       |
//                   |         |                                       |
//                   2 2 2 1 1 1 1 1 1 1 1 1 1                         |
// Register bits:    2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 <---
//                   |   |       |     |   |       |     |   |
// OSC3 bits  :      7   6       5     4   3       2     1   0
//
// Since waveform output is 12 bits the output is left-shifted 4 times.
//

func (w *WaveformGenerator) outputN___() reg12 {

	res := ((w.shiftreg & 0x400000) >> 11) |
		((w.shiftreg & 0x100000) >> 10) |
		((w.shiftreg & 0x010000) >> 7) |
		((w.shiftreg & 0x002000) >> 5) |
		((w.shiftreg & 0x000800) >> 4) |
		((w.shiftreg & 0x000080) >> 1) |
		((w.shiftreg & 0x000010) << 1) |
		((w.shiftreg & 0x000004) << 2)

	return (reg12(res))
}

// Combined waveforms:
// By combining waveforms, the bits of each waveform are effectively short
// circuited. A zero bit in one waveform will result in a zero output bit
// (thus the infamous claim that the waveforms are AND'ed).
// However, a zero bit in one waveform will also affect the neighboring bits
// in the output. The reason for this has not been determined.
//
// Example:
//
//             1 1
// Bit #       1 0 9 8 7 6 5 4 3 2 1 0
//             -----------------------
// Sawtooth    0 0 0 1 1 1 1 1 1 0 0 0
//
// Triangle    0 0 1 1 1 1 1 1 0 0 0 0
//
// AND         0 0 0 1 1 1 1 1 0 0 0 0
//
// Output      0 0 0 0 1 1 1 0 0 0 0 0
//
//
// This behavior would be quite difficult to model exactly, since the SID
// in this case does not act as a digital state machine. Tests show that minor
// (1 bit)  differences can actually occur in the output from otherwise
// identical samples from OSC3 when waveforms are combined. To further
// complicate the situation the output changes slightly with time (more
// neighboring bits are successively set) when the 12-bit waveform
// registers are kept unchanged.
//
// It is probably possible to come up with a valid model for the
// behavior, however this would be far too slow for practical use since it
// would have to be based on the mutual influence of individual bits.
//
// The output is instead approximated by using the upper bits of the
// accumulator as an index to look up the combined output in a table
// containing actual combined waveform samples from OSC3.
// These samples are 8 bit, so 4 bits of waveform resolution is lost.
// All OSC3 samples are taken with FREQ=0x1000, adding a 1 to the upper 12
// bits of the accumulator each cycle for a sample period of 4096 cycles.
//
// Sawtooth+Triangle:
// The sawtooth output is used to look up an OSC3 sample.
//
// Pulse+Triangle:
// The triangle output is right-shifted and used to look up an OSC3 sample.
// The sample is output if the pulse output is on.
// The reason for using the triangle output as the index is to handle ring
// modulation. Only the first half of the sample is used, which should be OK
// since the triangle waveform has half the resolution of the accumulator.
//
// Pulse+Sawtooth:
// The sawtooth output is used to look up an OSC3 sample.
// The sample is output if the pulse output is on.
//
// Pulse+Sawtooth+Triangle:
// The sawtooth output is used to look up an OSC3 sample.
// The sample is output if the pulse output is on.
//

func (w *WaveformGenerator) output__ST() reg12 {
	lut := reg12((*w.wave__ST)[w.output__S_()])
	return (lut << 4)
}

func (w *WaveformGenerator) output_P_T() reg12 {
	lut := reg12((*w.wave_P_T)[w.output___T()>>1])
	return ((lut << 4) & w.output_P__())
}

func (w *WaveformGenerator) output_PS_() reg12 {
	lut := reg12((*w.wave_PS)[w.output__S_()])
	return ((lut << 4) & w.output_P__())
}

func (w *WaveformGenerator) output_PST() reg12 {
	lut := reg12((*w.wave_PST)[w.output__S_()])
	return ((lut << 4) & w.output_P__())
}

// Combined waveforms including noise:
// All waveform combinations including noise output zero after a few cycles.
// NB! The effects of such combinations are not fully explored. It is claimed
// that the shift register may be filled with zeroes and locked up, which
// seems to be true.
// We have not attempted to model this behavior, suffice to say that
// there is very little audible output from waveform combinations including
// noise. We hope that nobody is actually using it.
//

func (w *WaveformGenerator) outputNxxx() reg12 {
	return 0
}

// ----------------------------------------------------------------------------
// Select one of 16 possible combinations of waveforms.
// ----------------------------------------------------------------------------

func (w *WaveformGenerator) Output() reg12 {
	// It may seem cleaner to use an array of member functions to return
	// waveform output; however a switch with inline functions is faster.

	switch w.waveform {
	case 0x0:
		return w.output____()
	case 0x1:
		return w.output___T()
	case 0x2:
		return w.output__S_()
	case 0x3:
		return w.output__ST()
	case 0x4:
		return w.output_P__()
	case 0x5:
		return w.output_P_T()
	case 0x6:
		return w.output_PS_()
	case 0x7:
		return w.output_PST()
	case 0x8:
		return w.outputN___()
	default:
		return w.outputNxxx()
	}
}

// ----------------------------------------------------------------------------
// Register functions.
// ----------------------------------------------------------------------------
func (w *WaveformGenerator) WriteFREQ_LO(freq_lo reg8) {
	w.freq = (w.freq & 0xff00) | reg16(freq_lo&0x00ff)
}

func (w *WaveformGenerator) WriteFREQ_HI(freq_hi reg8) {
	w.freq = ((reg16(freq_hi) << 8 & 0xff00) | (w.freq & 0x00ff))
}

func (w *WaveformGenerator) WritePW_LO(pw_lo reg8) {
	w.pw = (w.pw & 0xf00) | reg12(pw_lo&0x0ff)
}

func (w *WaveformGenerator) WritePW_HI(pw_hi reg8) {
	w.pw = ((reg12(pw_hi) << 8) & 0xf00) | (w.pw & 0x0ff)
}

func (w *WaveformGenerator) WriteCONTROL_REG(control reg8) {
	w.waveform = (control >> 4) & 0x0f
	w.ringmod = control & 0x04
	w.sync = control & 0x02

	test_next := control & 0x08

	// Test bit set.
	// The accumulator and the shift register are both cleared.
	// NB! The shift register is not really cleared immediately. It seems like
	// the individual bits in the shift register start to fade down towards
	// zero when test is set. All bits reach zero within approximately
	// $2000 - $4000 cycles.
	// This is not modeled. There should fortunately be little audible output
	// from this peculiar behavior.
	if test_next != 0 {
		w.accumulator = 0
		w.shiftreg = 0
	} else {
		// Test bit cleared.
		// The accumulator starts counting, and the shift register is reset to
		// the value 0x7ffff8.
		// NB! The shift register will not actually be set to this exact value if the
		// shift register bits have not had time to fade to zero.
		// This is not modeled.
		if w.test != 0 {
			w.shiftreg = 0x7ffff8
		}
	}

	w.test = test_next

	// The gate bit is handled by the EnvelopeGenerator.
}

func (w *WaveformGenerator) readOSC() reg8 {
	return reg8(w.Output() >> 4)
}
