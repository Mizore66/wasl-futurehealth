package main

import "math/rand"

// noisyDob3 keeps a true patient's real DOB identical across registrations.
// Its blank rate matches v2: p=0.30 gives 0.30*0.15 = 4.5% missing.
func noisyDob3(dob string, r *rand.Rand, p float64) string {
	if r.Float64() < p*.15 {
		return ""
	}
	return dob
}
