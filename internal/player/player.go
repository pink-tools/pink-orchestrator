package player

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

type Player struct {
	ctx *malgo.AllocatedContext
}

func New() (*Player, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio playback context init failed: %w", err)
	}
	return &Player{ctx: ctx}, nil
}

func (p *Player) Play(path string, volume float64) {
	go p.play(path, volume)
}

func (p *Player) play(path string, volume float64) {
	samples, sampleRate, channels, err := decodeAudio(path)
	if err != nil {
		return
	}

	for i, s := range samples {
		v := float64(s) * volume
		if v > math.MaxInt16 {
			v = math.MaxInt16
		} else if v < math.MinInt16 {
			v = math.MinInt16
		}
		samples[i] = int16(v)
	}

	buf := samplesToBytes(samples)

	var (
		mu      sync.Mutex
		offset  int
		silence bool
		once    sync.Once
		done    = make(chan struct{})
	)

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(channels)
	deviceConfig.SampleRate = sampleRate

	onSend := func(pOutput, _ []byte, _ uint32) {
		mu.Lock()
		defer mu.Unlock()

		if silence {
			clear(pOutput)
			once.Do(func() { close(done) })
			return
		}

		n := copy(pOutput, buf[offset:])
		offset += n
		if n < len(pOutput) {
			clear(pOutput[n:])
		}
		if offset >= len(buf) {
			silence = true
		}
	}

	device, err := malgo.InitDevice(p.ctx.Context, deviceConfig, malgo.DeviceCallbacks{Data: onSend})
	if err != nil {
		return
	}

	if err := device.Start(); err != nil {
		device.Uninit()
		return
	}

	<-done
	device.Stop()
	device.Uninit()
}

func (p *Player) Close() {
	if p.ctx != nil {
		p.ctx.Uninit()
		p.ctx.Free()
	}
}

func samplesToBytes(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(s))
	}
	return buf
}

func decodeAudio(path string) ([]int16, uint32, uint16, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, 0, err
	}

	ext := strings.ToLower(path)
	if strings.HasSuffix(ext, ".aiff") || strings.HasSuffix(ext, ".aif") {
		return decodeAIFF(data)
	}
	return decodeWAV(data)
}

func decodeWAV(data []byte) ([]int16, uint32, uint16, error) {
	if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("not a WAV file")
	}

	var sampleRate uint32
	var channels, bitsPerSample uint16
	var pcmData []byte

	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := binary.LittleEndian.Uint32(data[pos+4 : pos+8])
		pos += 8

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, 0, 0, fmt.Errorf("invalid fmt chunk")
			}
			channels = binary.LittleEndian.Uint16(data[pos+2 : pos+4])
			sampleRate = binary.LittleEndian.Uint32(data[pos+4 : pos+8])
			bitsPerSample = binary.LittleEndian.Uint16(data[pos+14 : pos+16])
		case "data":
			end := pos + int(chunkSize)
			if end > len(data) {
				end = len(data)
			}
			pcmData = data[pos:end]
		}

		pos += int(chunkSize)
		if pos%2 != 0 {
			pos++
		}
	}

	if pcmData == nil {
		return nil, 0, 0, fmt.Errorf("no data chunk")
	}

	return parsePCM(pcmData, bitsPerSample, binary.LittleEndian), sampleRate, channels, nil
}

func decodeAIFF(data []byte) ([]int16, uint32, uint16, error) {
	if len(data) < 12 || string(data[:4]) != "FORM" || string(data[8:12]) != "AIFF" {
		return nil, 0, 0, fmt.Errorf("not an AIFF file")
	}

	var sampleRate uint32
	var channels, bitsPerSample uint16
	var pcmData []byte

	pos := 12
	for pos+8 <= len(data) {
		chunkID := string(data[pos : pos+4])
		chunkSize := binary.BigEndian.Uint32(data[pos+4 : pos+8])
		pos += 8

		switch chunkID {
		case "COMM":
			channels = binary.BigEndian.Uint16(data[pos : pos+2])
			bitsPerSample = binary.BigEndian.Uint16(data[pos+6 : pos+8])
			sampleRate = extended80ToUint32(data[pos+8 : pos+18])
		case "SSND":
			ssndOffset := binary.BigEndian.Uint32(data[pos : pos+4])
			dataStart := pos + 8 + int(ssndOffset)
			end := pos + int(chunkSize)
			if end > len(data) {
				end = len(data)
			}
			if dataStart < end {
				pcmData = data[dataStart:end]
			}
		}

		pos += int(chunkSize)
		if pos%2 != 0 {
			pos++
		}
	}

	if pcmData == nil {
		return nil, 0, 0, fmt.Errorf("no SSND chunk")
	}

	return parsePCM(pcmData, bitsPerSample, binary.BigEndian), sampleRate, channels, nil
}

func parsePCM(data []byte, bitsPerSample uint16, order binary.ByteOrder) []int16 {
	switch bitsPerSample {
	case 16:
		samples := make([]int16, len(data)/2)
		for i := range samples {
			samples[i] = int16(order.Uint16(data[i*2:]))
		}
		return samples
	case 8:
		samples := make([]int16, len(data))
		for i, b := range data {
			samples[i] = int16(b-128) << 8
		}
		return samples
	case 24:
		samples := make([]int16, len(data)/3)
		for i := range samples {
			off := i * 3
			if order == binary.LittleEndian {
				samples[i] = int16(order.Uint16(data[off+1:]))
			} else {
				samples[i] = int16(order.Uint16(data[off:]))
			}
		}
		return samples
	default:
		return nil
	}
}

// extended80ToUint32 converts an 80-bit IEEE 754 extended precision float to uint32.
// Used for AIFF sample rate field.
func extended80ToUint32(b []byte) uint32 {
	exponent := int(binary.BigEndian.Uint16(b[:2])) & 0x7FFF
	mantissa := binary.BigEndian.Uint64(b[2:10])

	if exponent == 0 && mantissa == 0 {
		return 0
	}

	e := exponent - 16383 - 63
	if e >= 0 {
		return uint32(mantissa << uint(e))
	}
	return uint32(mantissa >> uint(-e))
}
