package main

// Private observation only. Work numbers are opaque process-local counters, NOT
// event numbers, pointers, owner tokens, hashes or source identities. c.mu owns
// the fixed ring and the scalar work annotations. No diagnostic lock nests I/O.
const headFrameLimit = 16

// Accounting: not observed, not returned, already applied, applied this turn,
// recovery, work/state position mismatch, work/state reference mismatch.
const (
	headAccountingNone uint8 = iota
	headNotReturned
	headAlreadyApplied
	headApplied
	headRecovery
	headPositionMismatch
	headReferenceMismatch
	headAccountingCount
)

// Canonical return sites; zero explicitly means no canonical call was observed.
const (
	headNoCanonical uint8 = iota
	headInvalidWork
	headBusyWork
	headUnavailable
	headContext
	headSource
	headCell
	headReceiptConflict
	headACKConflict
	headStoredReceipt
	headRetainedProof
	headFrozenConflict
	headAttemptSource
	headAttemptDisappeared
	headACKRead
	headHistoryRead
	headHistoryConflict
	headProofOnly
	headHistoryError
	headPrefix
	headNoWire
	headNewAttempt
	headOlderPrefix
	headNewFence
	headFenceCAS
	headFinalSource
	headAttemptWrite
	headAttemptAuthority
	headSendContext
	headWireReturned
	headWireUnknown
	headSiteCount
)

type headPredicate struct {
	Step    uint8  `json:"step"`  // helper-specific evaluated predicate, zero=not observed
	Equal   uint16 `json:"equal"` // source: sub/num/provider/focus/time/key/identity bits
	Present bool   `json:"present"`
}

type headHistory struct {
	Observed          bool   `json:"observed"`
	State             uint8  `json:"state"` // unknown/matching/absent/conflict = 1/2/3/4
	Current           bool   `json:"current"`
	Initialized       bool   `json:"initialized"`
	Invalid           bool   `json:"invalid"`
	RetentionLost     bool   `json:"retention_lost"`
	Conflicting       bool   `json:"conflicting"`
	First             uint64 `json:"first"`
	Last              uint64 `json:"last"`
	Msgs              uint64 `json:"msgs"`
	Traversed         uint64 `json:"traversed"`
	Covered           uint64 `json:"covered"`
	Capture           bool   `json:"capture"`
	Next              uint64 `json:"next"`
	End               uint64 `json:"end"`
	CaptureBusy       bool   `json:"capture_busy"`
	InfoBusy          bool   `json:"info_busy"`
	Version           uint64 `json:"version"`
	InfoStartObserved bool   `json:"info_start_observed"`
	InfoStart         uint64 `json:"info_start"`
	Blocker           uint64 `json:"blocker"`
	BlockerKind       uint8  `json:"blocker_kind"` // positive/transient/hole/malformed/unseen/conflict=1..6
	LocatorObserved   bool   `json:"locator_observed"`
	Locators          uint64 `json:"locators"`
	LocatorEqual      uint8  `json:"locator_equal"` // subject/body/length bits, first classified locator
}

type headFence struct {
	Step          uint8       `json:"step"` // input/slot/cell/original/marshal/update/zero/read/reconcile=1..9
	SlotPresent   bool        `json:"slot_present"`
	SlotValid     bool        `json:"slot_valid"`
	CurrentAbsent bool        `json:"current_absent"`
	PrefixEqual   bool        `json:"prefix_equal"`
	CellPresent   bool        `json:"cell_present"`
	RevisionEqual bool        `json:"revision_equal"`
	ValueEqual    bool        `json:"value_equal"`
	History       headHistory `json:"history"`
}

type canonicalTurnDiagnostic struct {
	Site             uint8         `json:"site"`
	Error            uint8         `json:"error"` // existing eight transport/error categories
	TokenEqual       bool          `json:"token_equal"`
	InputWireUnknown bool          `json:"input_wire_unknown"`
	InputProofOnly   bool          `json:"input_proof_only"`
	Owned            bool          `json:"owned"` // flags below observed only with canonical token ownership
	AllowWire        bool          `json:"allow_wire"`
	WireUnknown      bool          `json:"wire_unknown"`
	ProofOnly        bool          `json:"proof_only"`
	Wire             uint8         `json:"wire"` // unknown enum rejected; not-sent/fresh/unknown/duplicate=1/2/3/4
	Proof            bool          `json:"proof"`
	Hole             bool          `json:"hole"`
	Finish           uint8         `json:"finish"` // none/no-proof/receipt/binding/complete=0..4
	Source           headPredicate `json:"source"`
	Cell             headPredicate `json:"cell"`
	History          headHistory   `json:"history"`
	Fence            headFence     `json:"fence"`
}

type headAssociation struct {
	StatePresent  bool    `json:"state_present"`
	Recovery      bool    `json:"recovery"`
	ProviderOwned bool    `json:"provider_owned"`
	ProviderHead  bool    `json:"provider_head"`
	CurrentWork   bool    `json:"current_work"`
	Position      [3]bool `json:"position_equal"` // work/state-position, work/state-ref, state-position/ref
	ReturnApplied bool    `json:"return_applied"`
}

type headTurnFrame struct {
	Work          uint64                  `json:"work"`
	Turn          uint64                  `json:"turn"`
	Start         headAssociation         `json:"start"`
	AccountingCut headAssociation         `json:"accounting_cut"`
	TokenEqual    bool                    `json:"token_equal"`
	Returned      bool                    `json:"result_returned"`
	Applied       bool                    `json:"return_applied"`
	Accounting    uint8                   `json:"accounting"`
	Blocker       bool                    `json:"blocker"`
	Live          uint8                   `json:"live"` // initialization source/incarnation/persist/ack/work/canonical/floor/retire=1..8
	Canonical     canonicalTurnDiagnostic `json:"canonical"`
}

type headSelection struct {
	Work          uint64          `json:"work"`
	Started       uint64          `json:"started"`
	Finished      uint64          `json:"finished"`
	FrameRetained bool            `json:"frame_retained"` // false + Finished>0 means evicted, NOT no turn
	Ready         bool            `json:"ready"`
	Detached      uint8           `json:"detached"` // not in Q0/waiting/selected/unresolved=0..3
	Member        bool            `json:"known_task"`
	Queued        bool            `json:"queued"`
	Inflight      bool            `json:"inflight"`
	Selected      bool            `json:"selected"`
	Association   headAssociation `json:"association"`
}

type headProbeSnapshot struct {
	Scope         uint8                             `json:"scope"` // 1 = this observer/process only, not a durable identity
	ServiceActive bool                              `json:"service_active"`
	SelectedKind  uint8                             `json:"selected_kind"` // none/work/legacy/capture/retry/unknown=0..5
	SelectedWork  uint64                            `json:"selected_work"`
	Heads         [phaseProviderLimit]headSelection `json:"heads"` // same capped sorted registry as progress
	FrameCount    uint64                            `json:"frame_count"`
	Frames        []headTurnFrame                   `json:"frames"`
}

func (c *SubConsumer) headWorkIDLocked(w *pendingWork) uint64 {
	if c.phaseDiagnostics == nil || w == nil {
		return 0
	}
	if w.diagnosticID == 0 {
		c.phaseDiagnostics.headNextID++
		w.diagnosticID = c.phaseDiagnostics.headNextID
	}
	return w.diagnosticID
}
func (c *SubConsumer) headAssociationLocked(w *pendingWork) headAssociation {
	if w.state == nil {
		return headAssociation{Recovery: w.recovery, ProviderOwned: w.providerOwned, ProviderHead: c.providerHeads[w.event.Provider] == w, ReturnApplied: w.returnApplied}
	}
	return headAssociation{StatePresent: true, Recovery: w.recovery, ProviderOwned: w.providerOwned,
		ProviderHead: c.providerHeads[w.event.Provider] == w, CurrentWork: w.state.work[w.event.Num] == w,
		Position: [3]bool{w.positionRef == w.state.positionRef, w.positionRef == w.state.ref, w.state.positionRef == w.state.ref}, ReturnApplied: w.returnApplied}
}
func (c *SubConsumer) headStartLocked(w *pendingWork) (f headTurnFrame) {
	if d := c.phaseDiagnostics; d != nil {
		f.Work = c.headWorkIDLocked(w)
		w.diagnosticStarted++
		d.headTurn++
		f.Turn = d.headTurn
		f.Start = c.headAssociationLocked(w)
	}
	return
}
func (c *SubConsumer) headEndLocked(w *pendingWork, token uint64, f headTurnFrame) headTurnFrame {
	d := c.phaseDiagnostics
	if d == nil {
		return f
	}
	f.AccountingCut = c.headAssociationLocked(w)
	f.TokenEqual = w.attempt == token
	f.Returned = w.result.Result.Returned
	f.Blocker = w.blocker != ""
	switch {
	case !f.Returned:
		f.Accounting = headNotReturned
	case f.AccountingCut.ReturnApplied:
		f.Accounting = headAlreadyApplied
	case w.recovery:
		f.Accounting = headRecovery
	case !f.AccountingCut.Position[0]:
		f.Accounting = headPositionMismatch
	case !f.AccountingCut.Position[1]:
		f.Accounting = headReferenceMismatch
	default:
		f.Accounting = headApplied
	}
	// Called immediately BEFORE the original accounting condition. Its result is
	// recorded after that condition by headCommitLocked, never used to decide it.
	return f
}
func (c *SubConsumer) headCommitLocked(w *pendingWork, f headTurnFrame) {
	if d := c.phaseDiagnostics; d != nil {
		f.Applied = w.returnApplied
		d.headFrames[d.headCount%headFrameLimit] = f
		d.headCount++
		w.diagnosticFinished++
	}
}
func (c *SubConsumer) headSelectionLocked(w *pendingWork) (v headSelection) {
	if w == nil {
		return
	}
	d := c.phaseDiagnostics
	v.Work = c.headWorkIDLocked(w)
	v.Started = w.diagnosticStarted
	v.Finished = w.diagnosticFinished
	v.Detached = w.diagnosticDetached
	v.Member = c.knownTasks[canonicalTask{Kind: "work", Work: w}]
	v.Queued = w.queued
	v.Inflight = w.inflight
	v.Selected = d.headSelectedWork == v.Work
	v.Association = c.headAssociationLocked(w)
	for _, task := range c.knownWork {
		if task.Kind == "work" && task.Work == w {
			v.Ready = true
			break
		}
	}
	n := d.headCount
	if n > headFrameLimit {
		n = headFrameLimit
	}
	for i := d.headCount - n; i < d.headCount; i++ {
		if d.headFrames[i%headFrameLimit].Work == v.Work {
			v.FrameRetained = true
			break
		}
	}
	return
}
func (c *SubConsumer) headSelectLocked(task canonicalTask) {
	if d := c.phaseDiagnostics; d != nil {
		d.headSelectedWork = 0
		switch task.Kind {
		case "work":
			d.headSelectedKind = 1
			d.headSelectedWork = c.headWorkIDLocked(task.Work)
			task.Work.diagnosticDetached = 2
		case "legacy-seen":
			d.headSelectedKind = 2
		case "capture":
			d.headSelectedKind = 3
		case "sequence-retry":
			d.headSelectedKind = 4
		default:
			d.headSelectedKind = 5
		}
	}
}
func headHistoryState(s string) uint8 {
	switch s {
	case canonicalHistoryUnknown:
		return 1
	case canonicalHistoryMatching:
		return 2
	case canonicalHistoryAbsent:
		return 3
	case canonicalHistoryConflict:
		return 4
	}
	return 0
}
func headSlotKind(s string) uint8 {
	switch s {
	case canonicalSlotPositive:
		return 1
	case canonicalSlotTransient:
		return 2
	case canonicalSlotHole:
		return 3
	case canonicalSlotMalformed:
		return 4
	case "unseen":
		return 5
	case canonicalHistoryConflict:
		return 6
	}
	return 0
}
func headWireKind(s canonicalWireDisposition) uint8 {
	switch s {
	case canonicalWireNotSent:
		return 1
	case canonicalWireFresh:
		return 2
	case canonicalWireUnknown:
		return 3
	case canonicalWireDuplicate:
		return 4
	}
	return 0
}
func headPredicateStep(p []*headPredicate, step uint8) {
	if len(p) > 0 && p[0] != nil {
		p[0].Step = step
	}
}

// Read only ALREADY classified local values, under the existing history lock.
// This frame is carried with the return, not reread from a later mutable index.
func (s *canonicalHistoryIndex) headHistoryLocked(r canonicalHistoryResult, id canonicalIdentity, current bool) (v headHistory) {
	v = headHistory{Observed: true, State: headHistoryState(r.State), Current: current, Initialized: s.initialized, Invalid: s.invalid, RetentionLost: s.retentionLost, Conflicting: s.conflicting,
		First: s.observed.FirstSeq, Last: s.observed.LastSeq, Msgs: s.observed.Msgs, Traversed: s.traversed, Covered: s.covered, CaptureBusy: s.captureBusy, InfoBusy: s.infoBusy, Version: s.observation}
	if s.capture != nil {
		v.Capture = true
		v.Next = s.capture.Next
		v.End = s.capture.End
	}
	if len(r.Blockers) > 0 {
		v.Blocker = r.Blockers[0].Sequence
		v.BlockerKind = headSlotKind(r.Blockers[0].State)
	}
	locators := r.Candidates
	if r.State == canonicalHistoryConflict {
		locators = r.Blockers
	}
	if r.State == canonicalHistoryMatching || r.State == canonicalHistoryConflict {
		v.LocatorObserved = true
		v.Locators = uint64(len(locators))
		if len(locators) > 0 {
			a := locators[0]
			if a.Subject == id.Subject {
				v.LocatorEqual |= 1
			}
			if a.BodySHA256 == id.BodySHA256 {
				v.LocatorEqual |= 2
			}
			if a.BodyLength == id.BodyLength {
				v.LocatorEqual |= 4
			}
		}
	}
	return
}
func (v headProbeSnapshot) valid() bool {
	if v.Scope > 1 || v.SelectedKind > 5 || len(v.Frames) > headFrameLimit {
		return false
	}
	for _, h := range v.Heads {
		if h.Detached > 3 {
			return false
		}
	}
	for _, f := range v.Frames {
		c := f.Canonical
		h := c.History
		if f.Live > 8 || c.Fence.Step > 9 || f.Accounting >= headAccountingCount || c.Site >= headSiteCount || c.Error >= diagOutcomeCount || c.Wire > 4 || c.Finish > 4 || !headSourceStepValid(c.Source.Step) || c.Source.Equal > 127 || c.Cell.Step > 5 || !h.valid() || !c.Fence.History.valid() {
			return false
		}
	}
	return true
}

// Gaps are not known enum values, even though they fit in uint8.
func headSourceStepValid(s uint8) bool { return s <= 7 || s >= 9 && s <= 16 || s >= 20 && s <= 24 }
func (h headHistory) valid() bool      { return h.State <= 4 && h.BlockerKind <= 6 && h.LocatorEqual <= 7 }
