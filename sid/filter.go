package resid

import (
	"math"
)

const pi = 3.1415926535897932385

// Filter represents the filter in the SID chip.
type SidFilter struct {
	// Filter enabled.
	Enabled bool

	// Filter cutoff frequency.
	Fc reg12

	// Selects which inputs to route through filter.
	Res reg8

	// Selects which inputs to route through filter.
	Filter reg8

	// Switch voice 3 off.
	Voice3Off reg8

	// Highpass, bandpass, and lowpass filter modes.
	HpBpLp reg8

	// Output master volume.
	Volume reg4

	// Mixer DC offset.
	Mixer_DC sound_sample

	// State of filter.
	Vhp sound_sample // highpass
	Vbp sound_sample // bandpass
	Vlp sound_sample // lowpass
	Vnf sound_sample // not filtered

	// Cutoff frequency, resonance.
	w0, w0_ceil_1, w0_ceil_dt sound_sample
	_1024_div_Q               sound_sample

	// Lookup table ptr for f0, cutoff frequency
	F0 *[]int16
}

// ----------------------------------------------------------------------------
// Constructor.
// ----------------------------------------------------------------------------
func NewSidFilter() *SidFilter {
	f := &SidFilter{}

	f.Fc = 0
	f.Res = 0
	f.Filter = 0
	f.Voice3Off = 0
	f.HpBpLp = 0
	f.Volume = 0

	// State of SidFilter
	f.Vhp = 0
	f.Vbp = 0
	f.Vlp = 0
	f.Vnf = 0

	f.EnableFilter(true)
	f.SetModel(MOS6581)
	return f
}

func (f *SidFilter) Reset() {
	f.Fc, f.Res, f.Filter, f.Voice3Off = 0, 0, 0, 0
	f.Vhp, f.Vbp, f.Vlp, f.Vnf = 0, 0, 0, 0
	f.HpBpLp, f.Volume = 0, 0

	f.SetW0()
	f.SetQ()
}

// ----------------------------------------------------------------------------
// Register functions.
// ----------------------------------------------------------------------------
func (f *SidFilter) WriteFC_LO(fc_lo reg8) {
	f.Fc = (f.Fc & 0x7f8) | reg12(fc_lo&0x007)
	f.SetW0()
}

func (f *SidFilter) WriteFC_HI(fc_hi reg8) {
	f.Fc = ((reg12(fc_hi) << 3) & 0x7f8) | (f.Fc & 0x007)
	f.SetW0()
}

func (f *SidFilter) WriteRES_FILT(res_filt reg8) {
	f.Res = (res_filt >> 4) & 0x0f
	f.SetQ()

	f.Filter = res_filt & 0x0f
}

func (f *SidFilter) WriteMODE_VOL(mode_vol reg8) {
	f.Voice3Off = mode_vol & 0x80
	f.HpBpLp = (mode_vol >> 4) & 0x07
	f.Volume = reg4(mode_vol & 0x0f)
}

func (f *SidFilter) SetW0() {
	// Multiply with 1.048576 to facilitate division by 1 000 000 by right-
	// shifting 20 times (2 ^ 20 = 1048576).
	f.w0 = sound_sample(math.Round(2.0 * pi * float64((*f.F0)[f.Fc]) * 1.048576))

	// Limit f0 to 16kHz to keep 1 cycle SidFilter stable.
	w0_max_1 := sound_sample(math.Round(2.0 * pi * 16000.0 * 1.048576))

	if f.w0 <= w0_max_1 {
		f.w0_ceil_1 = f.w0
	} else {
		f.w0_ceil_1 = w0_max_1
	}

	// Limit f0 to 4kHz to keep delta_t cycle SidFilter stable.
	w0_max_dt := sound_sample(math.Round(2.0 * pi * 4000.0 * 1.048576))
	// w0_ceil_dt = w0 <= w0_max_dt ? w0 : w0_max_dt;
	if f.w0 <= w0_max_dt {
		f.w0_ceil_dt = f.w0
	} else {
		f.w0_ceil_dt = w0_max_dt
	}
}

func (f *SidFilter) SetQ() {
	// Q is controlled linearly by res. Q has approximate range [0.707, 1.7].
	// As resonance is increased, the SidFilter must be clocked more often to keep
	// stable.

	// The coefficient 1024 is dispensed of later by right-shifting 10 times
	// (2 ^ 10 = 1024).
	f._1024_div_Q = sound_sample(math.Round(1024.0 / (0.707 + 1.0*float64(f.Res)/15.0)))
}

func (f *SidFilter) EnableFilter(enable bool) {
	f.Enabled = enable
}

func (f *SidFilter) SetModel(model Model) {

	if model == MOS6581 {
		// The mixer has a small input DC offset. This is found as follows:
		//
		// The "zero" output level of the mixer measured on the SID audio
		// output pin is 5.50V at zero volume, and 5.44 at full
		// volume. This yields a DC offset of (5.44V - 5.50V) = -0.06V.
		//
		// The DC offset is thus -0.06V/1.05V ~ -1/18 of the dynamic range
		// of one voice. See voice.cc for measurement of the dynamic
		// range.

		f.Mixer_DC = -0xfff * 0xff / 18 >> 7
		f.F0 = &filter6581
	} else {
		// No DC offsets in the MOS8580.
		f.Mixer_DC = 0
		f.F0 = &filter8580
	}

	f.SetW0()
	f.SetQ()
}

func (f *SidFilter) Clock(delta_t CycleCount, voice1 sound_sample, voice2 sound_sample, voice3 sound_sample, ext_in sound_sample) {
	// Scale each voice down from 20 to 13 bits.
	voice1 >>= 7
	voice2 >>= 7

	// NB! Voice 3 is not silenced by voice3off if it is routed through
	// the filter.
	if f.Voice3Off != 0 && (f.Filter&0x04 == 0) {
		voice3 = 0
	} else {
		voice3 >>= 7
	}

	ext_in >>= 7

	// Enable filter on/off.
	// This is not really part of SID, but is useful for testing.
	// On slow CPUs it may be necessary to bypass the filter to lower the CPU
	// load.
	if !f.Enabled {
		f.Vnf = voice1 + voice2 + voice3 + ext_in
		f.Vhp, f.Vbp, f.Vlp = 0, 0, 0
		return
	}

	// Route voices into or around filter.
	// The code below is expanded to a switch for faster execution.
	// (filt1 ? Vi : Vnf) += voice1;
	// (filt2 ? Vi : Vnf) += voice2;
	// (filt3 ? Vi : Vnf) += voice3;
	var Vi sound_sample

	switch f.Filter {
	case 0x0:
		Vi = 0
		f.Vnf = voice1 + voice2 + voice3 + ext_in
	case 0x1:
		Vi = voice1
		f.Vnf = voice2 + voice3 + ext_in
	case 0x2:
		Vi = voice2
		f.Vnf = voice1 + voice3 + ext_in
	case 0x3:
		Vi = voice1 + voice2
		f.Vnf = voice3 + ext_in
	case 0x4:
		Vi = voice3
		f.Vnf = voice1 + voice2 + ext_in
	case 0x5:
		Vi = voice1 + voice3
		f.Vnf = voice2 + ext_in
	case 0x6:
		Vi = voice2 + voice3
		f.Vnf = voice1 + ext_in
	case 0x7:
		Vi = voice1 + voice2 + voice3
		f.Vnf = ext_in
	case 0x8:
		Vi = ext_in
		f.Vnf = voice1 + voice2 + voice3
	case 0x9:
		Vi = voice1 + ext_in
		f.Vnf = voice2 + voice3
	case 0xa:
		Vi = voice2 + ext_in
		f.Vnf = voice1 + voice3
	case 0xb:
		Vi = voice1 + voice2 + ext_in
		f.Vnf = voice3
	case 0xc:
		Vi = voice3 + ext_in
		f.Vnf = voice1 + voice2
	case 0xd:
		Vi = voice1 + voice3 + ext_in
		f.Vnf = voice2
	case 0xe:
		Vi = voice2 + voice3 + ext_in
		f.Vnf = voice1
	case 0xf:
		Vi = voice1 + voice2 + voice3 + ext_in
		f.Vnf = 0
	default:
	}

	// Maximum delta cycles for the filter to work satisfactorily under current
	// cutoff frequency and resonance constraints is approximately 8.
	var delta_t_flt CycleCount = 8

	for delta_t != 0 {
		if delta_t < delta_t_flt {
			delta_t_flt = delta_t
		}

		// delta_t is converted to seconds given a 1MHz clock by dividing
		// with 1 000 000. This is done in two operations to avoid integer
		// multiplication overflow.

		// Calculate filter outputs.
		// Vhp = Vbp/Q - Vlp - Vi;
		// dVbp = -w0*Vhp*dt;
		// dVlp = -w0*Vbp*dt;
		w0_delta_t := sound_sample(int(f.w0_ceil_dt) * int(delta_t_flt) >> 6)

		dVbp := sound_sample(w0_delta_t * f.Vhp >> 14)
		dVlp := sound_sample(w0_delta_t * f.Vbp >> 14)
		f.Vbp -= dVbp
		f.Vlp -= dVlp
		f.Vhp = (f.Vbp * f._1024_div_Q >> 10) - f.Vlp - Vi

		delta_t -= delta_t_flt
	}
}

func (f *SidFilter) Output() sound_sample {
	// This is handy for testing.
	if !f.Enabled {
		return (f.Vnf + f.Mixer_DC) * sound_sample(f.Volume)
	}

	// Mix highpass, bandpass, and lowpass outputs. The sum is not
	// weighted, this can be confirmed by sampling sound output for
	// e.g. bandpass, lowpass, and bandpass+lowpass from a SID chip.

	// The code below is expanded to a switch for faster execution.
	// if (hp) Vf += Vhp;
	// if (bp) Vf += Vbp;
	// if (lp) Vf += Vlp;

	var Vf sound_sample

	switch f.HpBpLp {
	case 0x0:
		Vf = 0

	case 0x1:
		Vf = f.Vlp

	case 0x2:
		Vf = f.Vbp

	case 0x3:
		Vf = f.Vlp + f.Vbp

	case 0x4:
		Vf = f.Vhp

	case 0x5:
		Vf = f.Vlp + f.Vhp

	case 0x6:
		Vf = f.Vbp + f.Vhp

	case 0x7:
		Vf = f.Vlp + f.Vbp + f.Vhp
	default:
	}

	// Sum non-filtered and filtered output.
	// Multiply the sum with volume.
	return (f.Vnf + Vf + f.Mixer_DC) * sound_sample(f.Volume)
}
