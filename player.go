package main

import (
	"fmt"
	"os"
	psid "yaspg/app/psid"
	resid "yaspg/app/sid"

	"github.com/beevik/go6502/cpu"
)

const PAL_FRAMERATE float64 = 50.0
const NTSC_FRAMERATE float64 = 60.0
const CLOCKFREQ uint32 = 985248
const SAMPLEFREQ uint32 = 22050

type SidPlayer struct {
	sid           *resid.Sid
	mem           *FlatMemoryWithNotification
	cpu           *cpu.CPU
	model         resid.Model
	songHeader    *psid.PSIDHeader
	currentSong   uint16
	isPlaying     bool
	isLoaded      bool
	isInitialized bool
	framePeriod   uint32
	frameRate     float64
	delta_t       uint32
	clockFreq     uint32
	sampleFreq    uint32
}

func NewSidPlayer() *SidPlayer {
	player := &SidPlayer{}
	player.frameRate = PAL_FRAMERATE
	player.clockFreq = CLOCKFREQ
	player.model = resid.MOS6581
	player.sampleFreq = SAMPLEFREQ
	player.framePeriod = player.clockFreq / uint32(player.frameRate)
	player.mem = NewFlatMemoryWithNotification()
	player.mem.AttachWriteNotifier(player)
	player.cpu = cpu.NewCPU(cpu.NMOS, player.mem)
	player.sid = resid.NewSID()
	return player
}

func (s *SidPlayer) Reset() {
	if s.isPlaying {
		s.Stop()
	}

	s.sid.Reset()
	s.isPlaying = false
}

func (s *SidPlayer) Init() {
	s.Reset()
	// audio init
	// set samplerate from obtained
	s.sid.SetSamplingParameters(float64(CLOCKFREQ), resid.SAMPLE_FAST, float64(SAMPLEFREQ))
	s.sid.SetModel(s.model)

	if s.model == resid.MOS6581 {
		fmt.Println("Sid model = 6581")
	} else {
		fmt.Println("Sid model = 8580")
	}
	s.isInitialized = true
}

func (s *SidPlayer) Load(fileName string) bool {
	if s.isPlaying {
		return false
	}

	s.songHeader = psid.NewPSID()

	file, err := os.Open(fileName)
	check(err)
	defer file.Close()

	err = s.songHeader.LoadHeader(file)
	check(err)
	s.songHeader.PrintHeader()

	// Load PSID data into cpu memory
	err = s.songHeader.LoadData(s.cpu, file)
	check(err)
	s.currentSong = s.songHeader.StartSong - 1
	s.isLoaded = true

	return true
}

func (s *SidPlayer) updateFramePeriod() {
	if !s.isLoaded {
		return
	}

	if (s.songHeader.Speed & (1 << s.currentSong)) != 0 {
		if s.cpu.Mem.LoadByte(0xdc05) != 0 {
			s.framePeriod = uint32(s.cpu.Mem.LoadByte(0xdc04)) + uint32(s.cpu.Mem.LoadByte(0xdc05))<<8
			return
		}
		s.frameRate = NTSC_FRAMERATE
	}

	s.framePeriod = s.clockFreq / uint32(s.frameRate)
}

func (s *SidPlayer) setSampleRate(freq uint32) {
	s.sampleFreq = freq
	s.isInitialized = false

	if s.isPlaying {
		s.Start()
	}
}

func (s *SidPlayer) setSIDModel(model resid.Model) {
	s.model = model
	s.isInitialized = false

	if s.isPlaying {
		s.Start()
	}
}

func (s *SidPlayer) nextTune() {
	s.playTune(uint16(s.currentSong + 1))
}

func (s *SidPlayer) playTune(num uint16) {
	s.currentSong = num
	if s.isPlaying {
		s.Start()
	}
}

func (s *SidPlayer) Start() {
	if !s.isInitialized {
		s.Init()
	} else {
		s.Reset()
	}

	s.sid.SetSamplingParameters(float64(s.clockFreq), resid.SAMPLE_FAST, float64(s.sampleFreq))
	s.delta_t = (s.clockFreq / s.sampleFreq)
	s.framePeriod = s.clockFreq / uint32(s.frameRate)

	s.cpu.Mem.StoreByte(0x01, 0x37)

	if s.currentSong >= s.songHeader.Songs {
		s.currentSong = 0
	}

	fmt.Printf("Playing subtune %d\n", s.currentSong)
	s.initCPU(s.songHeader.InitAddress, uint8(s.currentSong), 0, 0)
	instr := 0

	for !s.runCPU() {
		s.mem.StoreByte(0xD012, s.mem.LoadByte(0xD012)+1)

		if (s.cpu.Mem.LoadByte(0xD012) == 0) || (((s.cpu.Mem.LoadByte(0xD011) & 0x80) != 0) && (s.cpu.Mem.LoadByte(0xd012) >= 0x38)) {
			tmp := s.cpu.Mem.LoadByte(0xD011)
			tmp ^= 0x80
			s.cpu.Mem.StoreByte(0xD011, tmp)
			s.cpu.Mem.StoreByte(0xD012, 0x0)
		}
		instr += 1

		if instr > int(MAX_INSTR) {
			fmt.Println("Warning: CPU executed a high number of instructions in init, breaking")
			break
		}
	}

	if s.songHeader.PlayAddress == 0 {
		fmt.Println("Warning: SID has play address 0, reading from interrupt vector instead")
		if s.cpu.Mem.LoadByte(0x01)&0x07 == 0x5 {
			s.songHeader.PlayAddress = uint16(s.cpu.Mem.LoadByte(0xFFFE)) | (uint16(s.cpu.Mem.LoadByte(0xFFFF)) << 8)
		} else {
			s.songHeader.PlayAddress = uint16(s.cpu.Mem.LoadByte(0x314)) | (uint16(s.cpu.Mem.LoadByte(0x315)) << 8)
		}
		fmt.Printf("New play address is $%04X\n", s.songHeader.PlayAddress)
	}

	s.updateFramePeriod()

	speedflag := (s.songHeader.Speed&(1<<s.currentSong) != 0)
	fmt.Printf("cpu_clk: %d[Hz] samplerate: %d[Hz] samples/frame: %d frame period: %d[us] delta_t: %d timing: %t\n",
		s.clockFreq, s.sampleFreq, s.framePeriod/s.delta_t, s.framePeriod, s.delta_t, speedflag)

	// audio_start();
	s.isPlaying = true
}

func (s *SidPlayer) Stop() {
	if !s.isPlaying {
		return
	}

	// audio_stop()
	s.isPlaying = false
}

func (s *SidPlayer) initCPU(newpc uint16, newa uint8, newx uint8, newy uint8) {
	s.cpu.SetPC(newpc)
	s.cpu.Reg.X = newx
	s.cpu.Reg.Y = newy
	s.cpu.Reg.A = newa
}

// Run CPU one step. Returns true if playroutine
// completed for this iteration. False otherwise.
func (s *SidPlayer) runCPU() bool {
	s.cpu.Step()

	// Peek at the next opcode at the current PC
	opcode := s.cpu.Mem.LoadByte(s.cpu.Reg.PC)

	// Look up the instruction data for the opcode
	inst := s.cpu.InstSet.Lookup(opcode)

	switch {
	case (inst.Opcode == 0x00):
		return true
	case (inst.Opcode == 0x40) && (s.cpu.Reg.SP == 0xFF):
		return true
	case (inst.Opcode == 0x60) && (s.cpu.Reg.SP == 0xFF):
		return true
	default:
		return false
	}
}

func (s *SidPlayer) Tick() {
	// Run the playroutine
	instr := 0
	s.initCPU(s.songHeader.PlayAddress, 0, 0, 0)

	for !s.runCPU() {
		instr += 1

		if instr > int(MAX_INSTR) {
			fmt.Println("Warning: CPU executed a high number of instructions in init, breaking")
			break
		}

		// Test for jump into Kernal interrupt handler exit
		if ((s.cpu.Mem.LoadByte(0x01) & 0x07) != 0x5) && (s.cpu.Reg.PC == 0xEA31 || s.cpu.Reg.PC == 0xEA81) {
			break
		}
	}

	// // check timing, update samples_per_frame as needed.
	if (s.cpu.Mem.LoadByte(1)&3 != 0) && (s.songHeader.Speed&(1<<s.currentSong) > 0) {
		s.framePeriod = (uint32(s.cpu.Mem.LoadByte(0xdc05)) << 8) | uint32(s.cpu.Mem.LoadByte(0xdc04))
	}

}

func (s *SidPlayer) Quit() {
	// audio_quit()
}

// OnWrite is called when the CPU has written to a memory location.
func (s *SidPlayer) OnWrite(addr uint16, v byte) {
	if addr >= 0xD400 && addr <= 0xD418 {
		// fmt.Printf("Sid reg update %X=%X\n", addr, v)
		s.sid.Write(uint8(addr-0xD400), v)
	}
}

type WriteNotification interface {
	OnWrite(addr uint16, v byte)
}

// FlatMemory represents an entire 16-bit address space as a singular
// 64K buffer.
type FlatMemoryWithNotification struct {
	b           [64 * 1024]byte
	writeNotify WriteNotification
}

// AttachWriteNotifier attaches a handler that is called whenever a store
// to memory operation is happening.
func (m *FlatMemoryWithNotification) AttachWriteNotifier(handler WriteNotification) {
	m.writeNotify = handler
}

// FlatMemorySidHooks creates a new 16-bit memory space with writes
// to Resid component too.
func NewFlatMemoryWithNotification() *FlatMemoryWithNotification {
	mem := FlatMemoryWithNotification{}
	return &mem
}

// LoadByte loads a single byte from the address and returns it.
func (m *FlatMemoryWithNotification) LoadByte(addr uint16) byte {
	return m.b[addr]
}

// LoadBytes loads multiple bytes from the address and returns them.
func (m *FlatMemoryWithNotification) LoadBytes(addr uint16, b []byte) {

	if int(addr)+len(b) <= len(m.b) {
		copy(b, m.b[addr:])
	} else {
		r0 := len(m.b) - int(addr)
		r1 := len(b) - r0
		copy(b, m.b[addr:])
		copy(b[r0:], make([]byte, r1))
	}
}

// LoadAddress loads a 16-bit address value from the requested address and
// returns it.
//
// When the address spans 2 pages (i.e., address ends in 0xff), the low
// byte of the loaded address comes from a page-wrapped address.  For example,
// LoadAddress on $12FF reads the low byte from $12FF and the high byte from
// $1200. This mimics the behavior of the NMOS 6502.
func (m *FlatMemoryWithNotification) LoadAddress(addr uint16) uint16 {
	if (addr & 0xff) == 0xff {
		return uint16(m.b[addr]) | uint16(m.b[addr-0xff])<<8
	}
	return uint16(m.b[addr]) | uint16(m.b[addr+1])<<8
}

// StoreByte stores a byte at the requested address.
func (m *FlatMemoryWithNotification) StoreByte(addr uint16, v byte) {
	m.b[addr] = v

	if m.writeNotify != nil {
		m.writeNotify.OnWrite(addr, v)
	}
}

// StoreBytes stores multiple bytes to the requested address.
func (m *FlatMemoryWithNotification) StoreBytes(addr uint16, b []byte) {
	copy(m.b[addr:], b)
}

// StoreAddress stores a 16-bit address value to the requested address.
func (m *FlatMemoryWithNotification) StoreAddress(addr uint16, v uint16) {
	m.b[addr] = byte(v & 0xff)
	if (addr & 0xff) == 0xff {
		m.b[addr-0xff] = byte(v >> 8)
	} else {
		m.b[addr+1] = byte(v >> 8)
	}
}
