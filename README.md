# yet-another-sidplayer-go
A SID player written in Go!

I wanted to learn more about Go and decided to port the resid library by Dag Lem written that exists in the public domain in C++, to Go. This project will most certainly evolve as I get more proficient in Go. :)

For the 6502 chip emulation, I did not recreate this but rather used an existing Go library found here:
https://github.com/beevik/go6502

Thanks to the author for providing this, seems to work nicely!

I have used the SDL2 library to enable playback in MacOs.

It seems to work pretty well at this point, however, feel free to come up with suggestions for improvements!

KNOWN LIMITATION(S)
- This code will only play regular PSID tunes. No RSID support is provided for in the code at this point.  My code will default to using the standard VBI interrupt timing to run the emulation and calculate the samples for the current window before the next interrupt hits. Basic support has been added to accommodate CIA based timings. To support pulsewidth and volume based playback of samples, a more elaborate scheme using cycle exact timings would have to be deployed.


Enjoy!
