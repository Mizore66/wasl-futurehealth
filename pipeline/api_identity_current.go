package main

import (
	"encoding/json"
	"fmt"
)

type currentIdentity struct {
	keep     map[string]bool
	rejected map[[2]string]bool
}

func registrationPair(a, b string) [2]string {
	if a > b {
		a, b = b, a
	}
	return [2]string{a, b}
}

func validRegistration(pk string) bool { return pk != "" && pk != "<nil>" }

func weakestEdge(b Block) (string, string, bool) {
	we, ok := b.Body["weakest_edge"].(map[string]interface{})
	if !ok {
		return "", "", false
	}
	a, c := fmt.Sprint(we["a"]), fmt.Sprint(we["b"])
	return a, c, validRegistration(a) && validRegistration(c)
}

func (s *apiServer) rejectedRegistrationPairs(caseID string) map[[2]string]bool {
	rejected := map[[2]string]bool{}
	// Cannot-links are registration-global and persistent in LinkerCore. A later
	// same, held confirmation, messaging answer or API-ledger compensation does
	// not undo one. The API review overlay never changes canonical membership.
	consider := func(so *Signoff) {
		if CheckSignoff(so, DualMerge) == nil && !secondIDPurpose(so.Purpose) && so.Hold == "" &&
			so.Link == "different" && validRegistration(so.Pair[0]) && validRegistration(so.Pair[1]) {
			rejected[registrationPair(so.Pair[0], so.Pair[1])] = true
		}
	}
	materialized := map[string]bool{}
	for i := range s.idSignoffs {
		consider(&s.idSignoffs[i])
		materialized[s.idSignoffs[i].ItemID] = true
	}
	// Older fixtures may persist full sign-offs directly in the optional ledger.
	// Never recover a missing purpose or pair by guessing from a question/loop.
	for _, signoffs := range s.signoffs {
		for _, raw := range signoffs {
			var so Signoff
			if json.Unmarshal(canon(raw), &so) == nil && !materialized[so.ItemID] {
				consider(&so)
			}
		}
	}
	return rejected
}

// currentIdentityFor uses the same hash-bound canonical cluster as person reads.
// Older runs have no cluster artifact: retain their recorded trusted links across
// clinical evidence replacements, applying canonical cannot-links conservatively.
// Neither path infers identity from demographics or from a winning report alone.
func (s *apiServer) currentIdentityFor(c *apiCase) *currentIdentity {
	if c.last == nil {
		return nil
	}
	rejected := s.rejectedRegistrationPairs(c.ID)
	if projection, err := s.loadPersonProjection(); err == nil {
		if cluster := projection.byCase[c.ID]; cluster != nil {
			keep := map[string]bool{}
			for _, registration := range cluster.registrations {
				keep[registration] = true
			}
			return &currentIdentity{keep: keep, rejected: rejected}
		}
	}
	ref := ""
	for _, id := range s.ids[c.ID] {
		if id["role"] == "referral" {
			ref = id["registration"]
		}
	}
	if ref == "" {
		return nil
	}
	nodes := map[string]bool{ref: true}
	edges := [][2]string{}
	for _, d := range c.decisions {
		for _, b := range d.Chain {
			if b.Type != "link_decision" || b.Body["near_miss"] == true {
				continue
			}
			if a, c, ok := weakestEdge(b); ok {
				nodes[a], nodes[c] = true, true
				edges = append(edges, [2]string{a, c})
			}
		}
	}
	for _, o := range c.last.Outreach {
		if validRegistration(o.Pair[1]) {
			nodes[o.Pair[1]] = true
			if validRegistration(o.Pair[0]) {
				edges = append(edges, o.Pair)
			}
		}
	}
	adj := map[string][]string{}
	affected := map[string]bool{}
	for _, e := range edges {
		if rejected[registrationPair(e[0], e[1])] {
			affected[e[0]], affected[e[1]] = true, true
			continue
		}
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}
	keep := map[string]bool{}
	seen := map[string]bool{}
	for start := range nodes {
		if seen[start] {
			continue
		}
		comp, queue := []string{start}, []string{start}
		seen[start] = true
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			for _, m := range adj[n] {
				if !seen[m] {
					seen[m] = true
					comp = append(comp, m)
					queue = append(queue, m)
				}
			}
		}
		hasRef, hasAffected := false, false
		for _, n := range comp {
			hasRef, hasAffected = hasRef || n == ref, hasAffected || affected[n]
		}
		if hasRef || !hasAffected {
			for _, n := range comp {
				keep[n] = true
			}
		}
	}
	return &currentIdentity{keep: keep, rejected: rejected}
}

func (ci *currentIdentity) linkIsCurrent(b Block, inLast bool) bool {
	if b.Body["near_miss"] == true {
		return false
	}
	if a, c, ok := weakestEdge(b); ok {
		return ci.keep[a] && ci.keep[c] && !ci.rejected[registrationPair(a, c)]
	}
	return inLast
}

// evidenceHistoryLabel separates the current clinical explanation from preserved
// evidence. Source membership wins even when a stale report remains in the latest
// chain; evidence replacement alone is supersession, never an identity split.
func (s *apiServer) evidenceHistoryLabel(c *apiCase, b Block, ci *currentIdentity, inLast bool) string {
	label := ""
	if !inLast {
		label = "superseded"
	}
	if ci != nil {
		report, _ := b.Body["report"].(string)
		for _, event := range s.facts[c.ID] {
			if report != "" && event["id"] == report {
				if registration, _ := event["patient_key"].(string); validRegistration(registration) {
					if !ci.keep[registration] {
						return "rejected_registration"
					}
					return label // a superseded path through another registration cannot reject this source
				}
			}
		}
		for _, d := range c.decisions {
			for _, link := range d.Chain {
				if link.Type == "link_decision" && report != "" && link.Body["downstream_event"] == report {
					if a, z, ok := weakestEdge(link); ok && (!ci.keep[a] || !ci.keep[z]) {
						return "rejected_registration"
					}
				}
			}
		}
	}
	return label
}

func (ci *currentIdentity) linkHistoryLabel(b Block) string {
	if a, c, ok := weakestEdge(b); ok && ci.rejected[registrationPair(a, c)] {
		return "rejected"
	}
	return "superseded"
}
