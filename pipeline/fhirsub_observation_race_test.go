//go:build race

package main

import "time"

// User-approved instrumented-test policy: retain all 4,098 actual publications
// and source/body/CAS assertions, but do not treat race instrumentation cost as
// the ordinary throughput SLA. Production contexts/deadlines are unchanged.
const pendingOwnerPublicationBudget = 12 * time.Second
const pendingOwnerRaceInstrumentation = true
