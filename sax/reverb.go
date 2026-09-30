package sax

// reverb is a tiny Schroeder-style reverb (4 combs + 2 allpasses).
func reverb(in []float64, mix float64) []float64 {
	combDelays := []int{1557, 1617, 1491, 1422}
	combFb := 0.78
	out := make([]float64, len(in))
	for _, d := range combDelays {
		buf := make([]float64, d)
		idx := 0
		for i, x := range in {
			y := buf[idx]
			buf[idx] = x + y*combFb
			idx = (idx + 1) % d
			out[i] += y * 0.25
		}
	}
	for _, d := range []int{225, 556} {
		buf := make([]float64, d)
		idx := 0
		for i := range out {
			x := out[i]
			y := buf[idx]
			buf[idx] = x + y*0.5
			out[i] = y - x*0.5
			idx = (idx + 1) % d
		}
	}
	res := make([]float64, len(in))
	for i := range in {
		res[i] = in[i]*(1-mix) + out[i]*mix
	}
	return res
}
