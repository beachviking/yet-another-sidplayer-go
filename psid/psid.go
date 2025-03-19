package psid

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/beevik/go6502/cpu"
)

type PSIDHeader struct {
	MagicID     [4]byte
	Version     uint16
	DataOffset  uint16
	LoadAddress uint16
	InitAddress uint16
	PlayAddress uint16
	Songs       uint16
	StartSong   uint16
	Speed       uint32
	Name        [32]byte
	Author      [32]byte
	Released    [32]byte
}

func NewPSID() *PSIDHeader {
	psid := &PSIDHeader{}
	return psid
}

func (psid *PSIDHeader) PrintHeader() {
	fmt.Printf("MagicID:  %s\n", psid.MagicID)
	fmt.Printf("Version:  %X\n", psid.Version)
	fmt.Printf("DataOffset:  0x%X\n", psid.DataOffset)
	fmt.Printf("LoadAddress: 0x%X\n", psid.LoadAddress)
	fmt.Printf("InitAddress: 0x%X\n", psid.InitAddress)
	fmt.Printf("PlayAddress: 0x%X\n", psid.PlayAddress)
	fmt.Printf("Songs: %d\n", psid.Songs)
	fmt.Printf("Startsong: %d\n", psid.StartSong)
	fmt.Printf("Speed: 0x%X\n", psid.Speed)
	fmt.Printf("Name: %s\n", psid.Name)
	fmt.Printf("Author: %s\n", psid.Author)
	fmt.Printf("Copyright: %s\n", psid.Released)
}

func (psid *PSIDHeader) LoadHeader(file *os.File) error {
	binary.Read(file, binary.BigEndian, psid)

	if binary.BigEndian.Uint32(psid.MagicID[:]) != 0x50534944 {
		return errors.New("not a valid psid file")
	}

	file.Seek(int64(psid.DataOffset), 0)
	if psid.LoadAddress == 0 {
		psid.LoadAddress = uint16(readByte(file)) | uint16(readByte(file))<<8
	}

	return nil
}

func (psid *PSIDHeader) LoadData(cpu *cpu.CPU, file *os.File) error {
	filePos, fileErr := file.Seek(0, io.SeekCurrent)
	check(fileErr)
	loadPos := filePos
	filePos, fileErr = file.Seek(0, io.SeekEnd)
	check(fileErr)
	loadEnd := uint16(filePos)
	loadSize := uint16(loadEnd) - uint16(loadPos)
	file.Seek(int64(loadPos), 0)

	if loadSize+psid.LoadAddress >= 0x10000-1 {
		return errors.New("SID data continues past end of C64 memory")
	}

	memPos := uint16(psid.LoadAddress)

	for {
		var b byte

		fileErr = binary.Read(file, binary.LittleEndian, &b)

		if fileErr == io.EOF {
			break
		}
		cpu.Mem.StoreByte(memPos, b)
		// fmt.Printf("Adr %04X val %02X\n", memPos, b)
		memPos++
	}

	return nil
}

func readByte(f *os.File) byte {
	var res byte
	err := binary.Read(f, binary.LittleEndian, &res)
	check(err)
	return res
}

// func readWord(f *os.File) uint16 {
// 	var res [2]byte
// 	err := binary.Read(f, binary.LittleEndian, &res)
// 	check(err)
// 	word := uint16(res[0])<<8 | uint16(res[1])
// 	return word
// }

func check(e error) {
	if e != nil {
		panic(e)
	}
}
