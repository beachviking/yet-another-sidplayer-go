package resid

// Filter represents the filter in the SID chip.
type ExternalFilter struct {
	mixer_DC sound_sample

	// State of filters.
	Vlp, Vhp, Vo sound_sample

	// Cutoff frequencies.
	w0lp, w0hp sound_sample

	enabled bool
}

// ----------------------------------------------------------------------------
// Constructor.
// ----------------------------------------------------------------------------
func NewExternalFilter() *ExternalFilter {
	f := &ExternalFilter{}
  f.Reset()
  f.EnableFilter(true)
  f.SetSamplingParameter(15915.6)
  f.SetModel(MOS6581)
  // f.SetModel(MOS8580)
  return f
}

func (f *ExternalFilter) Reset() {
	// State of filter.
	f.Vlp = 0
	f.Vhp = 0
	f.Vo = 0
}

func (e *ExternalFilter) EnableFilter(enable bool) {
	e.enabled = enable
}

// ----------------------------------------------------------------------------
// Setup of the external filter sampling parameters.
// ----------------------------------------------------------------------------
func (f *ExternalFilter) SetSamplingParameter(pass_freq float64) {
//   static const float pi = 3.1415926535897932385;

  // Low-pass:  R = 10kOhm, C = 1000pF; w0l = 1/RC = 1/(1e4*1e-9) = 100000
  // High-pass: R =  1kOhm, C =   10uF; w0h = 1/RC = 1/(1e3*1e-5) =    100
  // Multiply with 1.048576 to facilitate division by 1 000 000 by right-
  // shifting 20 times (2 ^ 20 = 1048576).

  f.w0hp = 105
  f.w0lp = sound_sample(pass_freq * (2.0 * pi * 1.048576))
  if (f.w0lp > 104858) {
    f.w0lp = 104858;
  }
}


// ----------------------------------------------------------------------------
// Set chip model.
// ----------------------------------------------------------------------------

func (e *ExternalFilter) SetModel(model Model) {

	if model == MOS6581 {
		// Maximum mixer DC output level; to be removed if the external
		// filter is turned off: ((wave DC + voice DC)*voices + mixer DC)*volume
		// See voice.cc and filter.cc for an explanation of the values.
		e.mixer_DC = ((((0x800-0x380)+0x800)*0xff*3 - 0xfff*0xff/18) >> 7) * 0x0f
	} else {
		// No DC offsets in the MOS8580.
		e.mixer_DC = 0
	}
}

func (e *ExternalFilter) Clock(delta_t CycleCount, Vi sound_sample ) {
    // This is handy for testing.
    if (!e.enabled) {
        // Remove maximum DC level since there is no filter to do it.
        e.Vlp, e.Vhp = 0,0
        e.Vo = Vi - e.mixer_DC;
        return
    }

    // Maximum delta cycles for the external filter to work satisfactorily
    // is approximately 8.
    var delta_t_flt CycleCount  = 8

    for (delta_t != 0) {
        if (delta_t < delta_t_flt) {
            delta_t_flt = delta_t;
        }

        // delta_t is converted to seconds given a 1MHz clock by dividing
        // with 1 000 000.

        // Calculate filter outputs.
        // Vo  = Vlp - Vhp;
        // Vlp = Vlp + w0lp*(Vi - Vlp)*delta_t;
        // Vhp = Vhp + w0hp*(Vlp - Vhp)*delta_t;

        dVlp := sound_sample(e.w0lp*sound_sample(delta_t_flt) >> 8)*(Vi - e.Vlp) >> 12
        dVhp := sound_sample(e.w0hp*sound_sample(delta_t_flt)*(e.Vlp - e.Vhp) >> 20)
        e.Vo = e.Vlp - e.Vhp;
        e.Vlp += dVlp;
        e.Vhp += dVhp;

        delta_t -= delta_t_flt;
    }
}

func (e *ExternalFilter) Output() sound_sample {
	return e.Vo
}
