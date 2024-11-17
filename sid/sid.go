package resid

type Sid struct {
	voice           [3]*Voice
	filter          *SidFilter
	extfilter       *ExternalFilter
	potx            reg8
	poty            reg8
	busValue        reg8
	busValueTTL     CycleCount
	clkFreq         float64
	extIn           sound_sample
	cyclesPerSample CycleCount
	sampleOffset    CycleCount
}

var cntdwn int = 100000

func NewSID() *Sid {
	sid := &Sid{}
	sid.voice[0] = NewVoice()
	sid.voice[1] = NewVoice()
	sid.voice[2] = NewVoice()

	sid.voice[0].SetSyncSource(sid.voice[2])
	sid.voice[1].SetSyncSource(sid.voice[0])
	sid.voice[2].SetSyncSource(sid.voice[1])

	sid.filter = NewSidFilter()
	sid.extfilter = NewExternalFilter()

	// set_sampling_parameters(985248, SAMPLE_FAST, AUDIO_SAMPLE_RATE_EXACT);
	sid.SetSamplingParameters(985248, SAMPLE_FAST, 22050)

	sid.busValue = 0
	sid.busValueTTL = 0

	sid.extIn = 0

	return sid
}

func (s *Sid) Reset() {
	s.voice[0].Reset()
	s.voice[1].Reset()
	s.voice[2].Reset()

	s.filter.Reset()
	s.extfilter.Reset()

	s.busValue = 0
	s.busValueTTL = 0
}

// ----------------------------------------------------------------------------
// Set chip model.
// ----------------------------------------------------------------------------

func (s *Sid) SetModel(model Model) {
	s.voice[0].SetModel(model)
	s.voice[1].SetModel(model)
	s.voice[2].SetModel(model)

	s.filter.SetModel(model)
	s.extfilter.SetModel(model)
}

// ----------------------------------------------------------------------------
// Write 16-bit sample to audio input.
// NB! The caller is responsible for keeping the value within 16 bits.
// Note that to mix in an external audio signal, the signal should be
// resampled to 1MHz first to avoid sampling noise.
// ----------------------------------------------------------------------------
func (s *Sid) Input(sample sound_sample) {
	// Voice outputs are 20 bits. Scale up to match three voices in order
	// to facilitate simulation of the MOS8580 "digi boost" hardware hack.
	s.extIn = (sample << 4) * 3
}

// ----------------------------------------------------------------------------
// Read sample from audio output.
// ----------------------------------------------------------------------------
func (s *Sid) Output() int {
	rng := int(1 << 16)
	half := int(rng >> 1)
	//var sample int = int(s.extfilter.Output()) / ((4095 * 255 >> 7) * 3 * 15 * 2 / rng)
	var sample int = int(s.extfilter.Output())
	sample /= ((4095 * 255 >> 7) * 3 * 15 * 2 / rng)
	if sample >= half {
		return half - 1
	}
	if sample < -half {
		return -half
	}
	return sample
}

// ----------------------------------------------------------------------------
// Read registers.
//
// Reading a write only register returns the last byte written to any SID
// register. The individual bits in this value start to fade down towards
// zero after a few cycles. All bits reach zero within approximately
// $2000 - $4000 cycles.
// It has been claimed that this fading happens in an orderly fashion, however
// sampling of write only registers reveals that this is not the case.
// NB! This is not correctly modeled.
// The actual use of write only registers has largely been made in the belief
// that all SID registers are readable. To support this belief the read
// would have to be done immediately after a write to the same register
// (remember that an intermediate write to another register would yield that
// value instead). With this in mind we return the last value written to
// any SID register for $2000 cycles without modeling the bit fading.
// ----------------------------------------------------------------------------
func (s *Sid) Read(offset uint8) uint8 {
	switch offset {
	case 0x19:
		return uint8(s.potx)
	case 0x1a:
		return uint8(s.poty)
	case 0x1b:
		return uint8(s.voice[2].Wave.readOSC())
	case 0x1c:
		return uint8(s.voice[2].Envelope.readENV())
	default:
		return uint8(s.busValue)
	}
}

// ----------------------------------------------------------------------------
// Write registers.
// ----------------------------------------------------------------------------
func (s *Sid) Write(offset uint8, val uint8) {
	value := reg8(val)
	s.busValue = reg8(value)
	s.busValueTTL = 0x2000

	switch offset {
	case 0x00:
		s.voice[0].Wave.WriteFREQ_LO(value)
	case 0x01:
		s.voice[0].Wave.WriteFREQ_HI(value)
	case 0x02:
		s.voice[0].Wave.WritePW_LO(value)
	case 0x03:
		s.voice[0].Wave.WritePW_HI(value)
	case 0x04:
		s.voice[0].WriteCONTROL_REG(value)
	case 0x05:
		s.voice[0].Envelope.WriteATTACK_DECAY(value)
	case 0x06:
		s.voice[0].Envelope.WriteSUSTAIN_RELEASE(value)
	case 0x07:
		s.voice[1].Wave.WriteFREQ_LO(value)
	case 0x08:
		s.voice[1].Wave.WriteFREQ_HI(value)
	case 0x09:
		s.voice[1].Wave.WritePW_LO(value)
	case 0x0a:
		s.voice[1].Wave.WritePW_HI(value)
	case 0x0b:
		s.voice[1].WriteCONTROL_REG(value)
	case 0x0c:
		s.voice[1].Envelope.WriteATTACK_DECAY(value)
	case 0x0d:
		s.voice[1].Envelope.WriteSUSTAIN_RELEASE(value)
	case 0x0e:
		s.voice[2].Wave.WriteFREQ_LO(value)
	case 0x0f:
		s.voice[2].Wave.WriteFREQ_HI(value)
	case 0x10:
		s.voice[2].Wave.WritePW_LO(value)
	case 0x11:
		s.voice[2].Wave.WritePW_HI(value)
	case 0x12:
		s.voice[2].WriteCONTROL_REG(value)
	case 0x13:
		s.voice[2].Envelope.WriteATTACK_DECAY(value)
	case 0x14:
		s.voice[2].Envelope.WriteSUSTAIN_RELEASE(value)
	case 0x15:
		s.filter.WriteFC_LO(value)
	case 0x16:
		s.filter.WriteFC_HI(value)
	case 0x17:
		s.filter.WriteRES_FILT(value)
	case 0x18:
		s.filter.WriteMODE_VOL(value)
	default:
	}
}

// ----------------------------------------------------------------------------
// SID voice muting.
// ----------------------------------------------------------------------------
func (s *Sid) Mute(channel reg8, enable bool) {
	// Only have 3 voices!
	if channel >= 3 {
		return
	}

	s.voice[channel].Mute(enable)
}

func (s *Sid) SetSamplingParameters(clock_freq float64, method SamplingMethod, sample_freq float64) bool {
	pass_freq := float64(-1)
	filter_scale := 0.97
	// The default passband limit is 0.9*sample_freq/2 for sample
	// frequencies below ~ 44.1kHz, and 20kHz for higher sample frequencies.
	if pass_freq < 0 {
		pass_freq = 20000
		if 2.0*pass_freq/sample_freq >= 0.9 {
			pass_freq = 0.9 * sample_freq / 2.0
		}
	} else {
		// Check whether the FIR table would overfill.
		if pass_freq > 0.9*sample_freq/2.0 {
			return false
		}
	}

	// The filter scaling is only included to avoid clipping, so keep
	// it sane.
	if filter_scale < 0.9 || filter_scale > 1.0 {
		return false
	}

	// Set the external filter to the pass freq
	s.extfilter.SetSamplingParameter(pass_freq)
	s.clkFreq = clock_freq
	//   s.sampling = method;

	s.cyclesPerSample =
		CycleCount(clock_freq/sample_freq*(1<<16) + 0.5)

	s.sampleOffset = 0
	//   s.sample_prev = 0;

	return true
}

// ----------------------------------------------------------------------------
// SID clocking - delta_t cycles.
// ----------------------------------------------------------------------------
func (s *Sid) Clock(delta_t CycleCount) {
	// int i;

	if delta_t <= 0 {
		return
	}

	// Age bus value.
	s.busValueTTL -= delta_t
	if s.busValueTTL <= 0 {
		s.busValue = 0
		s.busValueTTL = 0
	}

	// Clock amplitude modulators.
	s.voice[0].Envelope.Clock(delta_t)
	s.voice[1].Envelope.Clock(delta_t)
	s.voice[2].Envelope.Clock(delta_t)

	// Clock and synchronize oscillators.
	// Loop until we reach the current cycle.
	delta_t_osc := delta_t
	for delta_t_osc > 0 {
		delta_t_min := delta_t_osc

		// Find minimum number of cycles to an oscillator accumulator MSB toggle.
		// We have to clock on each MSB on / MSB off for hard sync to operate
		// correctly.
		for i := 0; i < 3; i++ {
			wave := s.voice[i].Wave

			// It is only necessary to clock on the MSB of an oscillator that is
			// a sync source and has freq != 0.
			if !(wave.syncDest.sync != 0 && wave.freq != 0) {
				continue
			}

			freq := wave.freq
			accumulator := wave.accumulator

			// Clock on MSB off if MSB is on, clock on MSB on if MSB is off.
			var delta_accumulator reg24
			if accumulator&0x800000 != 0 {
				delta_accumulator = 0x1000000 - accumulator
			} else {
				delta_accumulator = 0x800000 - accumulator
			}

			delta_t_next := CycleCount(delta_accumulator / reg24(freq))
			if (delta_accumulator % reg24(freq)) != 0 {
				delta_t_next++
			}

			if delta_t_next < delta_t_min {
				delta_t_min = delta_t_next
			}
		}

		// Clock oscillators.
		s.voice[0].Wave.Clock(delta_t_min)
		s.voice[1].Wave.Clock(delta_t_min)
		s.voice[2].Wave.Clock(delta_t_min)

		// Synchronize oscillators.
		s.voice[0].Wave.Synchronize()
		s.voice[1].Wave.Synchronize()
		s.voice[2].Wave.Synchronize()

		delta_t_osc -= delta_t_min
	}

	// Clock filter.
	s.filter.Clock(delta_t, s.voice[0].Output(), s.voice[1].Output(), s.voice[2].Output(),
		s.extIn)

	// Clock external filter.
	s.extfilter.Clock(delta_t, s.filter.Output())

	// if cntdwn > 0 {
	// 	cntdwn--
	// 	fmt.Printf("%d v0=%d ve0=%d vw0=%d v1=%d ve1=%d vw1=%d v2=%d ve2=%d vw2=%d f=%d ef=%d sid=%d\n", cntdwn,
	// 		s.voice[0].Output(), s.voice[0].Envelope.Output(), s.voice[0].Wave.Output(),
	// 		s.voice[1].Output(), s.voice[1].Envelope.Output(), s.voice[1].Wave.Output(),
	// 		s.voice[2].Output(), s.voice[2].Envelope.Output(), s.voice[2].Wave.Output(),
	// 		s.filter.Output(), s.extfilter.Output(), s.Output())
	// }
}
