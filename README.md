# yet-another-sidplayer-go
A SID player written in Go!

As you may/may not know the siddump tool is a command line utility which takes a SID file and outputs the SID registers along with note
information. SID is the sound chip used in the Commodore 64.

I wanted to learn more about Go and decided to port the excellent siddump tool that exists in the public domain in C, to Go. This project will most certainly evolve as I get more proficient in Go. :)

Feel free to come up with suggestions for improvements.

For the 6502 chip emulation, I did not recreate this but rather used an existing Go library found here:
https://github.com/beevik/go6502

Thanks to the author for providing this, seems to work nicely!

Enjoy!
