package main

// might be useful to look at binary dumps in the terminal:
// od -h sidtune.dmp | less

// typedef unsigned char Uint8;
// void OnAudioCallback(void *userdata, Uint8 *stream, int len);
import "C"

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"unsafe"
	resid "yaspg/app/sid"

	"github.com/veandco/go-sdl2/sdl"
)

var (
	opt               *SidPlayerSettings
	player            *SidPlayer
	cpuplay_cnt_limit int = 882
	cpuplay_cnt       int
	dev               sdl.AudioDeviceID
)

const MAX_INSTR uint16 = 0xFFFF

//export OnAudioCallback
func OnAudioCallback(userdata unsafe.Pointer, stream *C.Uint8, length C.int) {

	n := int(length)
	hdr := reflect.SliceHeader{Data: uintptr(unsafe.Pointer(stream)), Len: n, Cap: n}
	buf := *(*[]C.Uint8)(unsafe.Pointer(&hdr))

	// Main calculation loop
	for i := 0; i < n; i += 4 {
		// Execute 6510 play routine if due
		cpuplay_cnt++
		if cpuplay_cnt >= cpuplay_cnt_limit {
			cpuplay_cnt = 0
			player.Tick()
			if player.framePeriod == 0 {
				player.framePeriod = 20000
			}
			cpuplay_cnt_limit = int(player.sampleFreq) / (int(player.clockFreq) / int(player.framePeriod))
		}

		// Calculate sid output
		player.sid.Clock(resid.CycleCount(player.delta_t))
		sample := (player.sid.Output())

		// Write to audio output buffer
		sampleHi := (sample >> 8)
		sampleLo := (sample & 0xFF)
		buf[i] = C.Uint8(sampleLo)
		buf[i+1] = C.Uint8(sampleHi)
		buf[i+2] = C.Uint8(sampleLo)
		buf[i+3] = C.Uint8(sampleHi)
	}
}

func main() {
	opt = NewSidPlayerSettings()
	player = NewSidPlayer()

	// Parse arguments
	opt.ParseArgs()

	if len(flag.Args()) == 0 {
		fmt.Println("Usage: go run main.go [options] <sidfile>")
		os.Exit(1)
	}

	if opt.Usage == 1 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// get file name of sid tune
	sidName := flag.Arg(0)

	player.setSampleRate(uint32(opt.Samplefreq))
	player.setSIDModel(resid.Model(opt.SidModel))
	player.Load(sidName)

	if err := sdl.Init(sdl.INIT_AUDIO); err != nil {
		log.Println(err)
		return
	}
	defer sdl.Quit()

	spec := &sdl.AudioSpec{}
	spec.Callback = sdl.AudioCallback(C.OnAudioCallback)
	spec.Samples = 4096
	spec.Channels = 2
	spec.Freq = int32(opt.Samplefreq)
	spec.Format = sdl.AUDIO_S16SYS

	var err error
	if dev, err = sdl.OpenAudioDevice("", false, spec, nil, 0); err != nil {
		log.Println(err)
		return
	}

	if opt.Subtune > -1 {
		player.currentSong = uint16(opt.Subtune)
	}

	player.Init()
	player.Start()

	sdl.PauseAudioDevice(dev, false)
	fmt.Println("Press the Enter Key to stop anytime")
	fmt.Scanln()
	sdl.CloseAudioDevice(dev)
	player.Stop()
}
