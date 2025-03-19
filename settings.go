package main

import "flag"

type SidPlayerSettings struct {
	Subtune int
	Samplefreq int
	SidModel int
	Usage   int
}

func NewSidPlayerSettings() *SidPlayerSettings {
	opt := &SidPlayerSettings{}
	return opt
}

func (opt *SidPlayerSettings) ParseArgs() {
	flag.IntVar(&opt.Subtune, "a", -1, "Accumulator value on init (subtune number) default = -1")
	flag.IntVar(&opt.Usage, "h", 0, "Display usage information")
	flag.IntVar(&opt.Samplefreq, "s", 22050, "Playback audio frequency in Hz, default 22050.")
	flag.IntVar(&opt.SidModel, "m", 0, "Sid model to use, 0=6581, 1=8580, default=0")

	flag.Parse()
}
