package audio

import (
	"sync"
	"time"
)

// playoutConfig controls the adaptive jitter buffer.
type playoutConfig struct {
	Target    time.Duration // starting (and minimum) buffered audio before playing
	MaxTarget time.Duration // ceiling for the adaptive target after underruns
	Max       time.Duration // hard ceiling; beyond this the backlog is dropped
}

func playoutConfigForProfile(profile string) playoutConfig {
	switch NormalizeProfile(profile) {
	case audioProfileLowLatency:
		return playoutConfig{Target: 20 * time.Millisecond, MaxTarget: 80 * time.Millisecond, Max: 160 * time.Millisecond}
	case audioProfileMusic:
		return playoutConfig{Target: 60 * time.Millisecond, MaxTarget: 200 * time.Millisecond, Max: 400 * time.Millisecond}
	default:
		return playoutConfig{Target: 40 * time.Millisecond, MaxTarget: 150 * time.Millisecond, Max: 300 * time.Millisecond}
	}
}

const (
	playoutFadeIn       = 5 * time.Millisecond
	playoutFadeOut      = 3 * time.Millisecond
	playoutTargetStep   = 10 * time.Millisecond
	playoutRelaxAfter   = 10 * time.Second
	playoutRelaxStep    = 5 * time.Millisecond
	playoutMaxDriftAdj  = 0.003 // ±0.3%: inaudible pitch change
	playoutCorrectionS  = 2.0   // seconds to absorb a buffer-level error
	playoutAdjSmoothing = 0.05
)

// Playout is a thread-safe adaptive jitter buffer between the network and a
// WASAPI render endpoint. It is fed decoded float32 frames at the sender's
// rate and rendered at the device's rate through a continuous resampler whose
// ratio is nudged to hold the buffer at its target level. That absorbs clock
// drift between the two sound cards without dropping chunks, and network
// jitter without writing long silences. Underruns fade out and back in
// instead of clicking.
type Playout struct {
	mu  sync.Mutex
	cfg playoutConfig
	now func() time.Time

	srcRate, srcCh int
	q              []float32 // interleaved source frames; frame 0 is history
	pos            float64   // fractional read position in frames within q

	primed       bool
	gain         float32
	adj          float64
	target       int // adaptive target, in source frames
	lastUnderrun time.Time
	lastRelax    time.Time

	lastOut  []float32
	frameIn  []float32
	frameOut []float32

	underruns, overflows uint64
}

func newPlayout(cfg playoutConfig) *Playout {
	return &Playout{cfg: cfg, now: time.Now}
}

func (p *Playout) framesFor(d time.Duration) int {
	return int(int64(p.srcRate) * int64(d) / int64(time.Second))
}

// SetConfig applies a new buffering profile without dropping queued audio.
func (p *Playout) SetConfig(cfg playoutConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = cfg
	if p.srcRate > 0 {
		p.target = p.framesFor(cfg.Target)
	}
}

// SetSource declares the format of subsequently pushed audio. Changing it
// discards queued audio, which belongs to the previous format.
func (p *Playout) SetSource(rate, channels int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if rate == p.srcRate && channels == p.srcCh {
		return
	}
	p.srcRate, p.srcCh = rate, channels
	p.frameIn = make([]float32, channels)
	p.resetLocked()
	p.target = p.framesFor(p.cfg.Target)
}

// Reset drops queued audio and waits to re-prime.
func (p *Playout) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resetLocked()
}

func (p *Playout) resetLocked() {
	p.q = make([]float32, p.srcCh, 8192)
	p.pos = 1
	p.primed = false
	p.adj = 0
	if p.srcRate > 0 {
		p.target = p.framesFor(p.cfg.Target)
	}
}

func (p *Playout) levelLocked() int {
	if p.srcCh == 0 {
		return 0
	}
	l := len(p.q)/p.srcCh - int(p.pos) - 1
	if l < 0 {
		return 0
	}
	return l
}

// Push queues interleaved frames in the current source format.
func (p *Playout) Push(samples []float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.srcCh == 0 || len(samples) < p.srcCh {
		return
	}
	p.q = append(p.q, samples[:len(samples)/p.srcCh*p.srcCh]...)
	if max := p.framesFor(p.cfg.Max); max > 0 && p.levelLocked() > max {
		// Far behind (e.g. the network stalled then burst). Jump to the newest
		// audio at the target level; drift control alone would take too long.
		frames := len(p.q) / p.srcCh
		keep := p.target + 2
		start := frames - keep
		n := copy(p.q, p.q[start*p.srcCh:])
		p.q = p.q[:n]
		p.pos = 1
		p.gain = 0
		p.overflows++
	}
}

// Render fills out with frames*dstCh interleaved samples at dstRate.
// It never returns short: missing audio is rendered as (faded) silence.
func (p *Playout) Render(out []float32, frames, dstRate, dstCh int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out = out[:frames*dstCh]
	if len(p.lastOut) != dstCh {
		p.lastOut = make([]float32, dstCh)
	}
	if p.srcRate == 0 || dstRate == 0 {
		p.fadeTail(out, 0, dstRate, dstCh)
		return
	}
	now := p.now()
	level := p.levelLocked()
	if !p.primed {
		if level < p.target || level < 3 {
			p.fadeTail(out, 0, dstRate, dstCh)
			return
		}
		p.primed = true
		p.gain = 0
	}

	errSeconds := float64(level-p.target) / float64(p.srcRate)
	want := errSeconds / playoutCorrectionS
	if want > playoutMaxDriftAdj {
		want = playoutMaxDriftAdj
	} else if want < -playoutMaxDriftAdj {
		want = -playoutMaxDriftAdj
	}
	p.adj += (want - p.adj) * playoutAdjSmoothing
	step := float64(p.srcRate) / float64(dstRate) * (1 + p.adj)
	gainStep := float32(1) / float32(max(1, int(float64(dstRate)*playoutFadeIn.Seconds())))

	ch := p.srcCh
	qFrames := len(p.q) / ch
	for f := 0; f < frames; f++ {
		i := int(p.pos)
		if i+2 >= qFrames {
			p.underrunLocked(now)
			p.fadeTail(out, f, dstRate, dstCh)
			break
		}
		t := float32(p.pos - float64(i))
		for c := 0; c < ch; c++ {
			p.frameIn[c] = cubic(p.q[(i-1)*ch+c], p.q[i*ch+c], p.q[(i+1)*ch+c], p.q[(i+2)*ch+c], t)
		}
		p.frameOut = mapChannels(p.frameOut[:0], p.frameIn, ch, dstCh)
		if p.gain < 1 {
			p.gain += gainStep
			if p.gain > 1 {
				p.gain = 1
			}
		}
		for c := 0; c < dstCh; c++ {
			v := p.frameOut[c] * p.gain
			out[f*dstCh+c] = v
			p.lastOut[c] = v
		}
		p.pos += step
	}

	// Drop consumed frames, keeping one frame of history.
	if drop := int(p.pos) - 1; drop > 2048 || (drop > 0 && drop*ch*2 > len(p.q)) {
		n := copy(p.q, p.q[drop*ch:])
		p.q = p.q[:n]
		p.pos -= float64(drop)
	}

	if p.target > p.framesFor(p.cfg.Target) && now.Sub(p.lastUnderrun) > playoutRelaxAfter && now.Sub(p.lastRelax) > playoutRelaxAfter {
		p.target -= p.framesFor(playoutRelaxStep)
		if base := p.framesFor(p.cfg.Target); p.target < base {
			p.target = base
		}
		p.lastRelax = now
	}
}

func (p *Playout) underrunLocked(now time.Time) {
	p.underruns++
	p.primed = false
	p.lastUnderrun = now
	if ceiling := p.framesFor(p.cfg.MaxTarget); p.target < ceiling {
		p.target += p.framesFor(playoutTargetStep)
		if p.target > ceiling {
			p.target = ceiling
		}
	}
}

// fadeTail ramps the last emitted sample down to zero from frame `from`, then
// writes silence, so a starved stream ends without a click.
func (p *Playout) fadeTail(out []float32, from, dstRate, dstCh int) {
	fade := 1
	if dstRate > 0 {
		fade = max(1, int(float64(dstRate)*playoutFadeOut.Seconds()))
	}
	frames := len(out) / dstCh
	for f := from; f < frames; f++ {
		k := float32(0)
		if n := f - from; n < fade {
			k = 1 - float32(n+1)/float32(fade)
		}
		for c := 0; c < dstCh; c++ {
			out[f*dstCh+c] = p.lastOut[c] * k
		}
	}
	for c := range p.lastOut {
		p.lastOut[c] = 0
	}
}

// playoutStats is a snapshot for diagnostics.
type playoutStats struct {
	Underruns, Overflows uint64
	BufferedMs, TargetMs int
}

func (p *Playout) Stats() playoutStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := playoutStats{Underruns: p.underruns, Overflows: p.overflows}
	if p.srcRate > 0 {
		s.BufferedMs = p.levelLocked() * 1000 / p.srcRate
		s.TargetMs = p.target * 1000 / p.srcRate
	}
	return s
}
