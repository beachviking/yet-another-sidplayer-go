package main

import "flag"

type SidPlayerSettings struct {
	Subtune int
	Usage   int
}

func NewSidPlayerSettings() *SidPlayerSettings {
	opt := &SidPlayerSettings{}
	return opt
}

func (opt *SidPlayerSettings) ParseArgs() {
	flag.IntVar(&opt.Subtune, "a", 0, "Accumulator value on init (subtune number) default = 0")
	flag.IntVar(&opt.Usage, "h", 0, "Display usage information")
	flag.Parse()
}
