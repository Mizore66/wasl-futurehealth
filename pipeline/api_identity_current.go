package main

import "fmt"

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
	for _, so := range s.signoffs[caseID] {
		pair, _ := so["pair"].([]interface{})
		if len(pair) != 2 {
			continue
		}
		key := registrationPair(fmt.Sprint(pair[0]), fmt.Sprint(pair[1]))
		switch so["link"] {
		case "different":
			rejected[key] = true
		case "same":
			delete(rejected, key)
		}
	}
	return rejected
}

// currentIdentityFor derives the registrations that belong to the case's patient from the latest decision only,
// minus every component a reviewer "different" answer cut away from the referral registration. It returns nil
// when the case has no referral registration to anchor on, which leaves the projections unfiltered.
func (s *apiServer) currentIdentityFor(c *apiCase) *currentIdentity {
	if c.last == nil {
		return nil
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
	for _, b := range c.last.Chain {
		if b.Type != "link_decision" {
			continue
		}
		if a, c, ok := weakestEdge(b); ok {
			nodes[a], nodes[c] = true, true
			edges = append(edges, [2]string{a, c})
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
	rejected := s.rejectedRegistrationPairs(c.ID)
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
	if !inLast {
		return false
	}
	if a, c, ok := weakestEdge(b); ok {
		return ci.keep[a] && ci.keep[c]
	}
	return true
}

func (ci *currentIdentity) linkHistoryLabel(b Block) string {
	if a, c, ok := weakestEdge(b); ok && ci.rejected[registrationPair(a, c)] {
		return "rejected"
	}
	return "superseded"
}
