//go:build !race

package main

import "time"

// Keep the original uninstrumented bulk-publication throughput requirement and
// late-claim schedule. The race build has a separate correctness-observation policy.
const pendingOwnerPublicationBudget = 1500 * time.Millisecond
const pendingOwnerRaceInstrumentation = false
