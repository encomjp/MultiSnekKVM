package audio

// cubic is a 4-point Catmull-Rom interpolation between y1 and y2.
func cubic(y0, y1, y2, y3, t float32) float32 {
	a := -0.5*y0 + 1.5*y1 - 1.5*y2 + 0.5*y3
	b := y0 - 2.5*y1 + 2*y2 - 0.5*y3
	c := -0.5*y0 + 0.5*y2
	return ((a*t+b)*t+c)*t + y1
}

// streamResampler converts a continuous interleaved stream between sample
// rates. Unlike per-chunk conversion it keeps phase and history across calls,
// so chunk boundaries produce no clicks and no cumulative rounding drift.
type streamResampler struct {
	channels int
	step     float64   // input frames per output frame
	pos      float64   // read position in frames, relative to buf
	buf      []float32 // pending input; frame 0 is one frame of history
}

func newStreamResampler(srcRate, dstRate, channels int) *streamResampler {
	r := &streamResampler{channels: channels, step: float64(srcRate) / float64(dstRate)}
	r.reset()
	return r
}

func (r *streamResampler) reset() {
	r.buf = make([]float32, r.channels, 4096) // one silent history frame
	r.pos = 1
}

func (r *streamResampler) passthrough() bool { return r.step == 1 }

// process appends resampled output for in to dst.
func (r *streamResampler) process(dst, in []float32) []float32 {
	if r.passthrough() {
		return append(dst, in...)
	}
	ch := r.channels
	r.buf = append(r.buf, in...)
	frames := len(r.buf) / ch
	for {
		i := int(r.pos)
		if i+2 >= frames {
			break
		}
		t := float32(r.pos - float64(i))
		for c := 0; c < ch; c++ {
			dst = append(dst, cubic(r.buf[(i-1)*ch+c], r.buf[i*ch+c], r.buf[(i+1)*ch+c], r.buf[(i+2)*ch+c], t))
		}
		r.pos += r.step
	}
	// Keep one frame of history before the next read position.
	drop := int(r.pos) - 1
	if drop > 0 {
		if drop > frames {
			drop = frames
		}
		n := copy(r.buf, r.buf[drop*ch:])
		r.buf = r.buf[:n]
		r.pos -= float64(drop)
	}
	return dst
}
