package main

import (
	"fmt"
	"strconv"
)

// -linkfloor lowers only the candidate review floor (LinkReview). Merge (-thr), closure trust and the equal-EID
// contradiction clamp (fixed 0.2) are unchanged. Default 0.2 adds no argv and no policy suffix.
func validateLinkFloor(v float64) error {
	if !(v > 0 && v <= 0.2) {
		return fmt.Errorf("-linkfloor must be >0 and <=0.2, got %v", v)
	}
	return nil
}

func linkFloorArgs() []string {
	if LinkReview == 0.2 {
		return nil
	}
	return []string{"-linkfloor", strconv.FormatFloat(LinkReview, 'g', -1, 64)}
}

func linkFloorPolicySuffix() string {
	if LinkReview == 0.2 {
		return ""
	}
	return "+linkfloor=" + strconv.FormatFloat(LinkReview, 'g', -1, 64)
}
