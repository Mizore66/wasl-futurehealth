package main

// wsi: Wasl+Sovereign integration pipeline.
//   wsi gen          -out cohort/                     synthetic multi-provider FHIR cohort + truth + evidence labels
//   wsi orchestrate  -run run/x -speed S -chaos N     3-node NATS JetStream cluster + all services as OS processes (+ kill/restart chaos)
//   wsi audit        -run run/x                       replay FHIR log offline, diff vs live outputs (dups/losses), accuracy, dashboard
//   wsi producer|prep|linker|closure|reviewer|setup   individual services (spawned by orchestrate)
import (
	"bufio"
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: wsi gen|orchestrate|audit|producer|prep|linker|closure|reviewer|setup|serve|patient-link|backup-nats|restore-nats|verify-nats")
		os.Exit(2)
	}
	cmd := os.Args[1]
	if cmd == "mcf" { // min-cost flow solver for the booking lookahead (mcf.go): JSON lines on stdin/stdout
		runMCF(os.Stdin, os.Stdout)
		return
	}
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	data := fs.String("data", "../cohort", "cohort dir")
	wasl := fs.String("wasl", "../../wasl/data", "wasl data dir (persons, encoder, scorer)")
	out := fs.String("out", "../cohort", "")
	bundle := fs.String("bundle", "", "backup-nats output or restore-nats/verify-nats input JSON")
	run := fs.String("run", "../run/default", "run dir")
	patientCase := fs.String("case", "", "patient-link: existing synthetic CRC case id")
	patientTTL := fs.Duration("ttl", defaultPatientTTL, "patient-link: lifetime (maximum 24h)")
	prov := fs.String("prov", "", "")
	name := fs.String("name", "0", "")
	part := fs.Int("part", 0, "")
	P := fs.Int("P", 4, "closure partitions")
	threads := fs.Int("threads", 4, "")
	preps := fs.Int("preps", 3, "prep worker processes")
	speed := fs.Float64("speed", 0, "simulated days per wall second (0 = max)")
	chaos := fs.Int("chaos", 0, "kill/restart cycles")
	natsKill := fs.Bool("natskill", false, "also kill+restart one NATS node during the run")
	chaosSpan := fs.Float64("chaosspan", 0, "orchestrate: seconds over which the kill/restart cycles are spread (0 = default, 15 s; larger cohorts have a longer feed)")
	quietTO := fs.Duration("quiettimeout", 5*time.Minute, "orchestrate: stop waiting for quiescence after this long (larger cohorts need more)")
	seed := fs.Int64("seed", 2026, "")
	loops := fs.Int("loops", 1500, "")
	distract := fs.Int("distract", 700, "")
	replicas := fs.Int("replicas", 3, "")
	noReview := fs.Bool("noreview", false, "")
	reports := fs.String("reports", "../evidence/reports43.json", "")
	pathT := fs.String("pathology", "../evidence/pathology_templates.json", "")
	persons := fs.Int("persons", 0, "cap on persons used (0 = all eval persons)")
	cerv := fs.Int("cerv", 600, "cervical loops (v6)")
	ecPool := fs.String("ecpool", "../econsult/testset.json", "")
	tbPool := fs.String("tbpool", "../teachback/testset.json", "")
	revNoise := fs.Float64("revnoise", 0, "simulated reviewer error rate")
	revLat := fs.Float64("revlat", 0, "simulated reviewer median latency per item (ms, wall)")
	hold := fs.String("hold", "", "demo: loop id whose two-reader disagreement is held for the presenter")
	rerr := fs.Float64("rerr", 0, "v6: reviewer per-read error rate")
	lungWk := fs.Bool("lungworkup", true, "lung nodule > 8 mm: PET/CT or tissue result can close the loop (lungworkup.go)")
	lungSurv := fs.Bool("lungsurv", false, "lung nodule: a PET/CT low-uptake closure makes a surveillance CT due (child loop <loop>#surv); gen adds the surveillance CTs (lungsurv.go)")
	lungSurv2 := fs.Bool("lungsurv2", false, "lung nodule (needs -lungsurv): later surveillance CTs at about 1 and 2 years after a PET/CT low-uptake closure (child loops <loop>#surv2, #surv3; BTS 2015); gen adds those CTs (lungsurv.go)")
	lungSurvStable := fs.Bool("lungsurvstable", false, "lung nodule: the stable-CT surveillance child gets review items, truth and metrics (lungsurv.go stableSurvLoop); gen adds its CTs")
	handoff := fs.Bool("handoff", true, "finally rejected referral: hand-off packet to the referrer, act-by deadline and escalation (handoff.go)")
	refReject := fs.Bool("refreject", false, "gen: lung work-up and DR eye-clinic referrals can be rejected by the unit (hand-off packet truth; handoffext.go)")
	wrongEID := fs.Float64("wrongeid", 0, "gen (adversarial B8): share of registrations carrying a relative's Emirates ID (genadv.go; 0 = off)")
	wrongRef := fs.Float64("wrongref", 0, "gen (adversarial B9): share of referral intakes citing the wrong referral (genadv.go; 0 = off)")
	histClock := fs.Bool("histclock", false, "closure: judge look-back history (inputs arriving before day 0) at its arrival time, not at the day-0 clock (linker.go HistClock)")
	xrefLink := fs.Bool("xreflink", false, "hand-off: a rejected referral intake that cites the loop's referral but never linked by identity raises a link question (handoffref.go)")
	prepQ := fs.Bool("prepquery", true, "ungraded prep: query the endoscopy unit, escalate at 5 working days, clinician at day 15 (prepquery.go)")
	r1Review := fs.Bool("r1review", false, "CRC: a colonoscopy report where neither reader finds a caecum statement goes to a clinician (completion_undocumented) instead of an automatic inadequate (r1review.go)")
	errUng := fs.Float64("errungraded", ErrUngraded, "per-read error of the day-15 ungraded-prep clinician decision (assumed stress-test rate)")
	rdouble := fs.String("rdouble", "none", "v6: double review policy none|merge|merge+close")
	rseed := fs.Int64("rseed", 1, "v6: reviewer noise seed")
	dual := fs.Bool("dual", false, "v6: linker requires two distinct reads for a human merge")
	sweepOut := fs.String("sweepout", "../results/sweep.json", "v6: sweep output")
	role := fs.String("role", "", "v6: caller role (tamper verifier requires auditor)")
	quick := fs.Bool("quick", false, "v6: sweep only err 0/0.05, no double, 1 seed")
	sweepRho := fs.String("sweeprho", "0", "H7: comma-separated reviewer error correlations to sweep (e.g. 0,0.25,0.5,1)")
	rrho := fs.Float64("rrho", 0, "H7: reviewer error correlation rho (P a later read repeats the first read)")
	revBlind := fs.Bool("reviewblind", false, "H7 mitigation: blinded second read (removes -blindfactor of rho)")
	blindF := fs.Float64("blindfactor", DefaultReviewer.BlindFactor, "H7: share of rho removed by a blinded second read")
	revRotate := fs.Bool("reviewrotate", false, "H7 mitigation: reader-pair rotation (removes -rotatefactor of the remaining rho)")
	rotateF := fs.Float64("rotatefactor", DefaultReviewer.RotateFactor, "H7: share of the remaining rho removed by pair rotation")
	seniorAudit := fs.Float64("senioraudit", 0, "H7 mitigation: fraction of signed closing answers re-read by a senior (e.g. 0.02)")
	seniorErr := fs.Float64("seniorerr", DefaultReviewer.SeniorErrRel, "H7: senior audit error rate relative to -rerr")
	sweepMit := fs.String("sweepmit", "", "H7: comma-separated mitigation sets to sweep: double,blind,rotate,audit,all (empty = pre-mitigation sweep)")
	sweepDouble := fs.String("sweepdouble", "", "sweep: comma-separated double-review policies (overrides -sweepset's list)")
	sweepErr := fs.String("sweeperr", "", "sweep: comma-separated reviewer error rates (overrides -sweepset's list)")
	sweepJobs := fs.Int("sweepjobs", 5, "sweep: jobs replayed concurrently (1 = sequential, repeatable output)")
	norbac := fs.Bool("norbac", false, "v6: counterfactual sweep with RBAC disabled (err 0 only)")
	models := fs.String("models", "../models", "")
	nRegs := fs.Int("n", 1000000, "")
	mode := fs.String("mode", "ivf", "")
	kNN := fs.Int("k", 10, "")
	nlist := fs.Int("nlist", 1024, "")
	nprobe := fs.Int("nprobe", 16, "")
	rerank := fs.Int("rerank", 100, "")
	hM := fs.Int("M", 16, "")
	efc := fs.Int("efc", 100, "")
	efs := fs.Int("efs", 64, "")
	recallN := fs.Int("recalln", 0, "")
	exSample := fs.Int("exsample", 0, "")
	snapAt := fs.Int("snapat", 0, "")
	shuffle := fs.Bool("shuffle", false, "")
	qevery := fs.Int("qevery", 1, "")
	annMode = fs.String("ann", "exact", "linker blocking index: exact|ivf|ivfsq8|hnsw")
	annCB = fs.String("codebook", "", "IVF codebook path")
	source := fs.String("source", "file", "file = v5 ndjson producers | fhir = mock Malaffi + FHIR Subscription consumer")
	content := fs.String("content", "full-resource", "subscription payload: full-resource | id-only")
	hieAddr := fs.String("hie", fmt.Sprintf("127.0.0.1:%d", 28880+PortOff), "mock Malaffi listen addr")
	hookAddr := fs.String("hook", fmt.Sprintf("127.0.0.1:%d", 28890+PortOff), "consumer rest-hook listen addr")
	fDup := fs.Float64("fdup", 0.05, "mock fault: P(duplicate notification)")
	fReo := fs.Float64("freorder", 0.05, "mock fault: P(reordered notification)")
	fDrop := fs.Float64("fdrop", 0.03, "mock fault: P(dropped notification)")
	subKill := fs.Int("subkill", 1, "kill/restart cycles of the FHIR subscription consumer (source=fhir)")
	pollSpeed := fs.Float64("pollspeed", 0, "malaffi: also serve _history and $export, HIE clock at this many simulated days per wall second (source=fhirpoll)")
	lagFrac := fs.Float64("lagfrac", 0.02, "malaffi poll mode: fraction of versions visible only 0.1-2 days after their lastUpdated (late commits)")
	pollOverlap := fs.Float64("overlap", 0.5, "fhirpoll: simulated days re-read before the watermark on each poll")
	hl7Addr := fs.String("hl7", fmt.Sprintf("127.0.0.1:%d", 28870+PortOff), "HL7 v2 MLLP listener addr (source=hl7)")
	apiAddr := fs.String("addr", "127.0.0.1:8088", "serve: API listen address")
	hl7Drops := fs.Int("hl7drops", 2, "mock HIE: TCP connection drops mid-stream (source=hl7)")
	v7cal := fs.String("v7cal", "", "v7: stratified conservative calibrator json (enables v7 path p_cal)")
	v7guard := fs.Bool("v7guard", false, "v7: second-identifier guard on EID-chain / p_cal outreach")
	tauClose := fs.Float64("tauclose", 0, "v7: closure link tier under -v7cal (default 0.99)")
	evpol := fs.String("evpolicy", "v6", "evidence policy v6|v7|v7strict (v7strict: ungraded prep never closes)")
	sweepSet := fs.String("sweepset", "full", "v7: full|v7 (err 0,.02,.04,.06 x none,merge+close x 5 seeds)")
	policy := fs.String("policy", "", "named policy bundle: v6 | v7strict | v7ltt (sets -v7cal/-v7guard/-tauclose/-evpolicy)")
	lttGrid := fs.String("lttgrid", "0.999,0.995,0.99,0.98,0.95,0.90,0.80,0.70,0.50", "ltt: candidate closure link tiers, strict to loose")
	tauLTT := fs.Bool("taultt", false, "internal: -tauclose came from Learn-then-Test (forwarded to workers by v7ltt)")
	lttSel := fs.String("lttsel", "../pipeline/results/ltt/ltt_selection.json", "v7ltt: Learn-then-Test selection (operating_tau_close, with review)")
	lttProg := fs.Bool("lttprog", false, "ltt: per-programme resolution and counts, second_identifier_confirm tally (ltt.go; off = rows as before)")
	lttOut := fs.String("lttout", "../pipeline/results/ltt/counts.json", "ltt: output")
	linkFloor := fs.Float64("linkfloor", 0.2, "linker+closure: candidate review floor (0,0.2]; merge/closure thresholds unchanged")
	agentRecsPath := fs.String("agentrecs", "", "agentreplay only: cached evidence-investigator records (agent_recs.ndjson); never forwarded to live child processes")
	agentMode := fs.String("agentmode", "cached", "agentshadow only: cached (never calls a provider) | live (explicit opt-in: also needs WASL_AGENTSHADOW_LIVE=1 and WASL_LLM_*)")
	agentItems := fs.Int("agentitems", AgentShadowMaxItems, "agentshadow only: max items investigated (<= 20)")
	agentCalls := fs.Int("agentcalls", AgentShadowMaxCalls, "agentshadow only: max uncached provider requests, persisted across restarts (<= 40)")
	agentDeadline := fs.Duration("agentdeadline", 15*time.Minute, "agentshadow only: overall deadline for new uncached requests")
	agentProvider := fs.Duration("agentprovider", 45*time.Second, "agentshadow only: per provider request deadline")
	agentIdle := fs.Duration("agentidle", 20*time.Second, "agentshadow only: exit after every provider ended and this long without events")
	agentCache := fs.String("agentcache", "", "agentshadow only: provider response cache dir (default <run>/agent_shadow/cache)")
	agentWorker := fs.String("agentworker", "../scripts/agent_shadow_worker.py", "agentshadow only: investigator sidecar script")
	agentPython := fs.String("agentpython", "python3", "agentshadow only: python interpreter for the sidecar")
	linkFunnel := fs.String("linkfunnel", "", "linker: write opt-in candidate-funnel NDJSON to this path (diagnostics only)")
	split := fs.String("split", "eval", "gen: person split eval (held-out test) | dev (development only, not held-out validation) | cal (Learn-then-Test calibration) | all (every split, incl. model training persons: scale/throughput tests only, accuracy not held-out)")
	fs.Float64Var(&MDNamesakeRate, "mdnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic missing-date namesake hard negative (pairfeat7 training only)")
	fs.Float64Var(&RDNamesakeRate, "rdnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic same-real-date stranger hard negative (pairfeat7 training only)")
	fs.Float64Var(&MINamesakeRate, "minamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic middle-initial namesake hard negative (pairfeat9 training only)")
	fs.Float64Var(&XSNamesakeRate, "xsnamesakes", 0, "featdump only, with -nameidf: share of persons given a synthetic cross-script namesake hard negative (pairfeat6 training only)")
	fs.Float64Var(&FamilyNamesakeRate, "familynamesakes", 0, "featdump only: share of persons given a synthetic family-name namesake hard negative (training only)")
	fs.Float64Var(&NamesakeRate, "namesakes", 0, "featdump only: share of persons given a synthetic namesake hard negative (training only)")
	noise := fs.String("noise", "v1", "gen: record-corruption model v1 (training-time) | v2 (independent, held-out evaluation only) | v3 (reliable real DOB: same-person registrations never carry different real birth dates; pairfeat8 training and evaluation)")
	readerMode := fs.String("reader", "cached", "second evidence reader: cached (Gemma 26B answers) | live (local llama-server) | replay (ledger only)")
	readerURL := fs.String("reader-url", "http://127.0.0.1:8791", "local OpenAI-compatible reader endpoint")
	readerTimeout := fs.Duration("reader-timeout", 60*time.Second, "per-call reader timeout")
	readerRetries := fs.Int("reader-retries", 2, "bounded reader retries")
	imageMode := fs.String("image", "off", "third (image) evidence reader, hold-only: off | cached (models/image_verdicts.json) | live (image/serve.py) | replay")
	imageURL := fs.String("image-url", "http://127.0.0.1:8792", "image reader sidecar")
	imgPhotos := fs.Bool("imgphotos", false, "gen: attach synthetic caecal / mucosal photo references (HyperKvasir held-out test images) to colonoscopy reports")
	imgCache := fs.String("imgcache", "", "gen/image: image verdict cache (default <wasl>/image_verdicts.json)")
	lungN := fs.Int("lung", 0, "gen: incidental lung-nodule loops (Fleischner 2017); 0 = CRC/cervical cohort only (byte-identical)")
	drN := fs.Int("dr", 0, "gen: diabetic retinopathy screening loops (doh2024dr); 0 = no DR loops (other feeds byte-identical)")
	lungData := fs.String("lungdata", "", "gen/lungcache: lung CT corpus dir (default <reports dir>/../lung)")
	drRecallMsg := fs.Bool("drrecallmsg", false, "outreach: a resolved DR loop flagged recall_overdue (-drnext) gets a consent-gated recall message, kind dr_next_recall (outreachgaps.go)")
	rejMsg := fs.Bool("rejmsg", false, "outreach: a finally rejected lung work-up / DR eye-clinic referral gets a consent-gated patient message; CRC patient_informed counts its referral_rejected_next_steps intent (outreachgaps.go)")
	drSeenLink := fs.Bool("drseenlink", false, "DR next loop: a later eye-clinic note on an untrusted link raises a link question and counts as a visit only after a human 'same' (outreachgaps.go)")
	drNext := fs.Bool("drnext", false, "DR next loop: next due rescreen / clinic review on resolved DR decisions, recall_overdue flag; gen: follow-up eye-clinic visits (drnext.go)")
	drData := fs.String("drdata", "", "gen/drcache: diabetic retinopathy corpus dir(s), comma-separated for drcache (default <reports dir>/../dr)")
	breastN := fs.Int("breast", 0, "gen: breast screening recall loops (doh2025breast); 0 = no breast loops (other feeds byte-identical)")
	breastShare := fs.Bool("breastshare", false, "gen: when women with no other loop run out, breast loops continue with women who hold another programme's loop (genbreast.go)")
	ldctN := fs.Int("ldct", 0, "gen: lung cancer screening LDCT loops (acr2022lungrads); 0 = no LDCT loops (other feeds byte-identical)")
	ldctData := fs.String("ldctdata", "", "gen/ldctcache: LDCT corpus dir(s), comma-separated for ldctcache (default <reports dir>/../ldct)")
	breastData := fs.String("breastdata", "", "gen/breastcache: breast corpus dir(s), comma-separated for breastcache (default <reports dir>/../breast)")
	ifhasN := fs.Int("ifhas", 0, "gen: IFHAS abnormal-result follow-up loops (doh2025ifhas, sixth loop type); 0 = none (other feeds byte-identical)")
	ifhasData := fs.String("ifhasdata", "", "gen/ifhascache: IFHAS follow-up note corpus dir(s), comma-separated for ifhascache (default <reports dir>/../ifhas)")
	encFile := fs.String("encfile", "", "encembed: encoder weights json")
	inFile := fs.String("in", "", "encembed: input file, one name per line")
	modelSet := fs.String("modelset", "", "linkage model chain in the -wasl/-models dir: empty = defaults; X = encoder_X.json + lgb_stream_X.json + iso_X.json + iso_v7_X.json")
	gbmFile := fs.String("gbmfile", "", "featdump: also write the pair-scorer probability from this LightGBM json")
	idfSeeds := fs.String("idfseeds", "", "nameidf: comma-separated featdump seeds whose base registrations the rarity table counts")
	nameIDFPath := fs.String("nameidf", "", "featdump only: pairfeat6 name-rarity table (wsi nameidf output); adds pair features f30-f33 (pairfeat6.go)")
	shadow := fs.Bool("shadow", false, "pilot: shadow mode, every automatic closure is a suggestion confirmed by a clinician (pilot.go)")
	corrShort := fs.Bool("corrshort", true, "linker: a last-surname skeleton under 3 letters corroborates only when the first-name skeleton agrees too (v7.go)")
	linkConfirmDOB := fs.Bool("linkconfirmdob", false, "linker: with -linkconfirm, real birth dates more than a year apart also count as a contradiction (v7.go)")
	linkConfirm := fs.Bool("linkconfirm", true, "linker: a human 'same' (link or second-identifier answer) on a pair with contradicting sex or real birth dates needs a confirming review (v7.go)")
	demoGuard := fs.Bool("demoguard", false, "linker (default off): different real DOBs or different known sexes are hard conflicts (cannot-link); missing demographics block automatic merging and go to review; reviewers cannot override a conflict (demoguard.go)")
	eidGuard := fs.Bool("eidguard", false, "linker+closure (B8 hardening, post hoc, default off): equal Emirates IDs on contradicting registrations do not force a link, and no closure rides an Emirates-ID path to a contradicting person (eidguard.go)")
	secondIDConfirm := fs.Bool("secondidconfirm", true, "linker: with -linkconfirm, a second-identifier 'same' on a pair with contradicting sex or real birth dates needs a confirming review (second_identifier_confirm; v7.go)")
	progTpl := fs.Bool("progtpl", true, "outreach: lung and DR intents name their programme's approved template (lnfu, drsfu; rules.go)")
	replayWL := fs.Bool("replayworklist", true, "ltt/sweep: the offline review loop answers items in emission order with the live reviewer's worklists (sweep.go)")
	reuseLink := fs.Bool("reuselink", true, "review worklist: one link question per registration pair; other loops' queued twins are dropped once it is answered (v7.go)")
	reuseConfirm := fs.Bool("reuseconfirm", true, "outreach guard: a second-identifier confirmation counts for the same registration pair on other loops (v7.go)")
	eidSecond := fs.Bool("eidsecond", true, "outreach guard: a matching Emirates ID with agreeing name, sex and DOB counts as the second identifier (v7.go)")
	corrFuzzy := fs.Bool("corrfuzzy", true, "linker: an exact-DOB pair also corroborates when the last surname skeletons (4+ letters) are within one edit (v7.go)")
	corrShared := fs.Bool("corrshared", true, "linker: an exact-DOB pair also corroborates when the names share two distinct 3+ letter skeleton tokens (v7.go)")
	autoAudit := fs.Float64("autoaudit", 0, "pilot: fraction of automatic closures given a post-closure clinician audit (normal mode only; 0 = off)")
	UrduFix = true // retrained v3 encoder was trained with the Urdu/Persian normalisation fix
	fs.Parse(os.Args[2:])
	if err := validateNoise(cmd, *noise); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	if err := validateNamesakes(cmd, NamesakeRate); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	if err := validateNamesakeRate("-familynamesakes", cmd, FamilyNamesakeRate); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	if err := validateXSNamesakes(cmd, XSNamesakeRate, *nameIDFPath); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	for _, v := range []struct {
		flag string
		rate float64
	}{{"-mdnamesakes", MDNamesakeRate}, {"-rdnamesakes", RDNamesakeRate}, {"-minamesakes", MINamesakeRate}} {
		if err := validateIDFNamesakes(v.flag, cmd, v.rate, *nameIDFPath); err != nil {
			fmt.Fprintln(os.Stderr, "configuration error:", err)
			os.Exit(2)
		}
	}
	if *agentRecsPath != "" && cmd != "agentreplay" {
		fmt.Fprintln(os.Stderr, "configuration error: -agentrecs is only valid with the agentreplay command")
		os.Exit(2)
	}
	if err := validateLinkFloor(*linkFloor); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	LinkReview = *linkFloor
	LinkFunnelPath = *linkFunnel
	if *linkFunnel != "" {
		LinkFunnelArgs = []string{"-linkfunnel", *linkFunnel}
	}
	if err := validateNatsConfig(os.Getenv("WASL_NATS_URL"), os.Getenv("WASL_NATS_EXTERNAL")); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	PrepQuery, ErrUngraded = *prepQ, *errUng
	R1ReviewOn = *r1Review
	Handoff, LungWorkup, LungPETSurv = *handoff, *lungWk, *lungSurv
	LungPETSurv2, LungStableSurv = *lungSurv && *lungSurv2, *lungSurvStable
	XRefLinkQ = *xrefLink
	HistClock = *histClock
	RefRejectExt = *refReject
	WrongEIDRate, WrongRefRate = *wrongEID, *wrongRef
	CorrShared, CorrFuzzyLast, EIDSecondID, CorrShortSurname, ReuseIDConfirm = *corrShared, *corrFuzzy, *eidSecond, *corrShort, *reuseConfirm
	ReuseLinkAnswer, LinkContradictionConfirm, ReplayWorklist, ProgTemplates = *reuseLink, *linkConfirm, *replayWL, *progTpl
	DRNextOn = *drNext
	BreastShare = *breastShare
	DRRecallMsg, RejectedMsg, DRSeenLinkAsk = *drRecallMsg, *rejMsg, *drSeenLink
	LinkContradictionDOB = *linkConfirmDOB
	SecondIDContradictionConfirm = *secondIDConfirm
	EIDContraGuard = *eidGuard
	DemoGuard = *demoGuard
	LTTProg = *lttProg
	// lung-nodule programme: register its two providers in every process of a lung run (children inherit the env var)
	if (cmd == "gen" && *lungN > 0) || os.Getenv("WSI_LUNG_PROVIDERS") == "1" || fileExists(filepath.Join(*data, "fhir_mafraq-rad.ndjson")) {
		EnableLung()
		os.Setenv("WSI_LUNG_PROVIDERS", "1")
	}
	// diabetic retinopathy programme: same, after the lung providers
	if (cmd == "gen" && *drN > 0) || os.Getenv("WSI_DR_PROVIDERS") == "1" || fileExists(filepath.Join(*data, "fhir_eye-hosp.ndjson")) {
		EnableDR()
		os.Setenv("WSI_DR_PROVIDERS", "1")
	}
	// breast recall programme: same, after the DR providers
	if (cmd == "gen" && *breastN > 0) || os.Getenv("WSI_BREAST_PROVIDERS") == "1" || fileExists(filepath.Join(*data, "fhir_bau-hosp.ndjson")) {
		EnableBreast()
		os.Setenv("WSI_BREAST_PROVIDERS", "1")
	}
	// lung cancer screening (LDCT) programme: same, after the breast providers
	if (cmd == "gen" && *ldctN > 0) || os.Getenv("WSI_LDCT_PROVIDERS") == "1" || fileExists(filepath.Join(*data, "fhir_thx-hosp.ndjson")) {
		EnableLDCT()
		os.Setenv("WSI_LDCT_PROVIDERS", "1")
	}
	LDCTGenN, LDCTGenDir = *ldctN, *ldctData
	// IFHAS follow-up programme (ifhas.go): same, after the LDCT providers
	if (cmd == "gen" && *ifhasN > 0) || os.Getenv("WSI_IFHAS_PROVIDERS") == "1" || fileExists(filepath.Join(*data, "fhir_ifu-clinic.ndjson")) {
		EnableIfhas()
		os.Setenv("WSI_IFHAS_PROVIDERS", "1")
	}
	GenIfhasN, GenIfhasDir = *ifhasN, *ifhasData
	ModelSet = *modelSet
	if err := validateModelPolicy(ModelSet, *policy); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	}
	XScriptCap = xscriptCapFor(ModelSet) // xscript.go: cross-script review cap (pairfeat5, pairfeat6, pairfeat7)
	if tb, err := nameIDFFor(cmd, *nameIDFPath, ModelSet, *wasl); err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		os.Exit(2)
	} else {
		NameIDF = tb // pairfeat6.go: non-nil only for -modelset pairfeat6/pairfeat7 or featdump -nameidf
	}
	DualMerge = *dual
	switch *policy {
	case "v7strict":
		if *v7cal == "" {
			*v7cal = filepath.Join(*wasl, mf("iso_v7.json"))
		}
		*v7guard, *evpol = true, "v7strict"
		if *tauClose == 0 {
			// 0.99 > the 0.989 cap on uncorroborated model edges, so a name-only link can never auto-close a loop alone.
			// Was 0.98 until the final evaluation on the retrained chain found one wrong-patient closure at the cap
			// (pipeline/RESULTS.md, "Final code"); raised after that finding, disclosed as a post-hoc change.
			*tauClose = TauV7Strict
		}
	case "v7ltt": // v7strict with the closure tier chosen by Learn-then-Test (scripts/ltt_select.py), not by hand
		if *v7cal == "" {
			*v7cal = filepath.Join(*wasl, mf("iso_v7.json"))
		}
		*v7guard, *evpol = true, "v7strict"
		if *tauClose == 0 {
			*tauClose, TauFromLTT = lttTau(*lttSel), true
		}
	case "", "v6":
	default:
		panic("unknown -policy " + *policy)
	}
	if *v7cal != "" {
		V7Cal = LoadCal7(*v7cal)
	}
	ShadowMode, AutoAuditP = *shadow, *autoAudit
	V7Guard, EvPolicy = *v7guard, *evpol
	if *tauClose > 0 {
		TauClose = *tauClose
	}
	TauFromLTT = TauFromLTT || *tauLTT
	PolicyName = policyName() + linkFloorPolicySuffix()
	if EIDContraGuard {
		PolicyName += "+eidguard" // post-hoc B8 hardening, default off: stamped on every decision so lanes are never mixed
	}
	PolicyName += demoGuardPolicySuffix() // demoguard.go: "+demoguard" only when on
	if ModelSet != "" {
		PolicyName += "+models=" + ModelSet
	}
	NoiseModel = *noise
	GenSplit = *split
	ChaosSpan, QuietTimeout = *chaosSpan, *quietTO
	ReaderMode = *readerMode
	switch *readerMode {
	case "cached":
	case "live", "replay":
		absRunR, _ := filepath.Abs(*run)
		os.MkdirAll(absRunR, 0700)
		url := *readerURL
		if *readerMode == "replay" {
			url = "http://127.0.0.1:1" // nothing listens: ledger hits only, misses fail safe to review
		}
		LiveReader = NewReaderClient(url, *readerTimeout, *readerRetries, filepath.Join(absRunR, "reader_ledger.jsonl"))
		if *readerMode == "live" && (cmd == "orchestrate" || cmd == "readerwarm") && !LiveReader.Healthy() {
			fmt.Fprintf(os.Stderr, "reader: %s is not healthy; start it with: llama-server -m <gemma-3-4b-it-Q4_K_M.gguf> --port 8791 -c 8192 -ngl 99\n", url)
			os.Exit(3)
		}
	default:
		panic("unknown -reader " + *readerMode)
	}
	if *imgCache == "" {
		*imgCache = filepath.Join(*wasl, "image_verdicts.json")
	}
	switch *imageMode {
	case "off":
	case "cached":
		ImageR = LoadImageCache(*imgCache)
	case "live", "replay":
		c := LoadImageCache(*imgCache) // thresholds are frozen with the model; live answers go to the ledger
		absRunI, _ := filepath.Abs(*run)
		os.MkdirAll(absRunI, 0700)
		url := *imageURL
		if *imageMode == "replay" {
			url = "http://127.0.0.1:1"
		}
		ImageR = NewImageClient(*imageMode, url, filepath.Join(absRunI, "image_ledger.jsonl"), c.ThrLand, c.ThrPr)
		if *imageMode == "live" && cmd == "orchestrate" && !ImageR.Healthy() {
			fmt.Fprintf(os.Stderr, "image reader: %s is not healthy; start it with: python image/serve.py\n", url)
			os.Exit(3)
		}
	default:
		panic("unknown -image " + *imageMode)
	}
	if ImageR != nil {
		PolicyName += "+image=" + ImageR.Mode
	}
	ImgPhotos, ImgCachePath = *imgPhotos, *imgCache
	if *sweepSet == "v7" {
		SweepErrs, SweepDoubles, SweepSeeds = []float64{0, 0.02, 0.04, 0.06}, []string{"none", "merge+close"}, 5
	}
	v7Args := []string{"-evpolicy", EvPolicy, fmt.Sprintf("-v7guard=%v", V7Guard), "-tauclose", fmt.Sprint(TauClose), fmt.Sprintf("-taultt=%v", TauFromLTT)}
	v7Args = append(v7Args, linkFloorArgs()...)
	v7Args = append(v7Args, fmt.Sprintf("-prepquery=%v", PrepQuery), fmt.Sprintf("-r1review=%v", R1ReviewOn), fmt.Sprintf("-handoff=%v", Handoff), fmt.Sprintf("-xreflink=%v", XRefLinkQ), fmt.Sprintf("-lungworkup=%v", LungWorkup), fmt.Sprintf("-corrshared=%v", CorrShared), fmt.Sprintf("-corrfuzzy=%v", CorrFuzzyLast), fmt.Sprintf("-eidsecond=%v", EIDSecondID), fmt.Sprintf("-corrshort=%v", CorrShortSurname), fmt.Sprintf("-reuseconfirm=%v", ReuseIDConfirm), fmt.Sprintf("-reuselink=%v", ReuseLinkAnswer), fmt.Sprintf("-linkconfirm=%v", LinkContradictionConfirm), fmt.Sprintf("-linkconfirmdob=%v", LinkContradictionDOB), fmt.Sprintf("-secondidconfirm=%v", SecondIDContradictionConfirm), fmt.Sprintf("-replayworklist=%v", ReplayWorklist), fmt.Sprintf("-progtpl=%v", ProgTemplates), "-errungraded", fmt.Sprint(ErrUngraded))
	v7Args = append(v7Args, modelPolicyArgs(ModelSet, *policy)...)
	for i, on := range []bool{DRRecallMsg, RejectedMsg, DRSeenLinkAsk} { // outreachgaps.go: every child, only when on
		if on {
			v7Args = append(v7Args, []string{"-drrecallmsg", "-rejmsg", "-drseenlink"}[i])
		}
	}
	if EIDContraGuard { // B8 hardening (eidguard.go): every child, only when on
		v7Args = append(v7Args, "-eidguard")
	}
	v7Args = append(v7Args, demoGuardArgs()...) // strict demographic guard (demoguard.go): every child, only when on
	if DRNextOn {                               // DR next loop (drnext.go): every child, only when on
		v7Args = append(v7Args, "-drnext")
	}
	if HistClock { // linker and closure children, only when on
		v7Args = append(v7Args, "-histclock")
	}
	if LungPETSurv { // reaches every child only when on, so the child argument lists are unchanged when off
		v7Args = append(v7Args, "-lungsurv")
	}
	if LungPETSurv2 {
		v7Args = append(v7Args, "-lungsurv2")
	}
	if LungStableSurv {
		v7Args = append(v7Args, "-lungsurvstable")
	}
	if ShadowMode { // pilot switches reach every child (closure engines, reviewer, linker) only when on
		v7Args = append(v7Args, "-shadow")
	}
	if AutoAuditP > 0 {
		v7Args = append(v7Args, "-autoaudit", fmt.Sprint(AutoAuditP))
	}
	if *readerMode != "cached" {
		v7Args = append(v7Args, "-reader", *readerMode, "-reader-url", *readerURL, "-reader-timeout", readerTimeout.String(), "-reader-retries", fmt.Sprint(*readerRetries))
	}
	if *v7cal != "" {
		abs, _ := filepath.Abs(*v7cal)
		v7Args = append(v7Args, "-v7cal", abs)
	}
	if *imageMode != "off" {
		absC, _ := filepath.Abs(*imgCache)
		v7Args = append(v7Args, "-image", *imageMode, "-image-url", *imageURL, "-imgcache", absC)
	}
	rcfg := DefaultReviewer
	rcfg.Err, rcfg.Double, rcfg.Seed = *rerr, *rdouble, *rseed
	PilotAdjSeed = *rseed
	if *rerr > 0 { // report7 runs without -rerr: keep the assumed 4%
		PilotAdjErr = *rerr
	}
	if rcfg.Err == 0 && *revNoise > 0 { // v6-impact flag name kept as an alias
		rcfg.Err = *revNoise
	}
	rcfg.Rho, rcfg.Blind, rcfg.BlindFactor, rcfg.Rotate, rcfg.RotateFactor = *rrho, *revBlind, *blindF, *revRotate, *rotateF
	rcfg.SeniorAudit, rcfg.SeniorErrRel = *seniorAudit, *seniorErr
	SweepMitBase = rcfg
	if SweepMitBase.SeniorAudit == 0 {
		SweepMitBase.SeniorAudit = 0.02 // -sweepmit audit|all without -senioraudit: the designed 2% sample
	}
	if *sweepMit != "" {
		SweepMits = strings.Split(*sweepMit, ",")
	}
	if *sweepDouble != "" {
		SweepDoubles = strings.Split(*sweepDouble, ",")
	}
	if *sweepErr != "" {
		SweepErrs = nil
		for _, x := range strings.Split(*sweepErr, ",") {
			v, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				panic("bad -sweeperr " + x)
			}
			SweepErrs = append(SweepErrs, v)
		}
	}
	if *sweepJobs >= 1 {
		SweepPar = *sweepJobs
	}
	// H7 reviewer flags are forwarded to child processes only when set, so default runs keep their argv
	h7Args := []string{}
	if *rrho > 0 {
		h7Args = append(h7Args, "-rrho", fmt.Sprint(*rrho))
	}
	if *revBlind {
		h7Args = append(h7Args, "-reviewblind", "-blindfactor", fmt.Sprint(*blindF))
	}
	if *revRotate {
		h7Args = append(h7Args, "-reviewrotate", "-rotatefactor", fmt.Sprint(*rotateF))
	}
	if *seniorAudit > 0 {
		h7Args = append(h7Args, "-senioraudit", fmt.Sprint(*seniorAudit), "-seniorerr", fmt.Sprint(*seniorErr))
	}
	absRun, _ := filepath.Abs(*run)
	switch cmd {
	case "gen":
		runGen(*wasl, *out, *reports, *pathT, *seed, *loops, *distract, *persons, *cerv, loadPools(*ecPool, *tbPool), *lungN, *lungData, *drN, *drData, *breastN, *breastData)
	case "setup":
		setupStreams(*replicas, *P)
	case "backup-nats", "restore-nats", "verify-nats":
		if *bundle == "" {
			panic(cmd + " requires -bundle")
		}
		nc, js := connect(cmd)
		defer nc.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		var summary recoverySummary
		var err error
		switch cmd {
		case "backup-nats":
			summary, err = backupNATS(ctx, js, *bundle)
		case "restore-nats":
			summary, err = restoreNATS(ctx, js, *bundle)
		case "verify-nats":
			summary, err = verifyNATS(ctx, js, *bundle)
		}
		if err != nil {
			writeStructuredLog("error", "NATS recovery operation failed", "operation", cmd, "error", err.Error())
			panic(err)
		}
		writeStructuredLog("info", "NATS recovery operation complete", "operation", cmd, "streams", summary.Streams, "messages", summary.Messages)
		encoded, _ := json.Marshal(map[string]interface{}{"status": "ok", "operation": cmd, "summary": summary,
			"nondeterministic_fields": []string{"nats_sequence", "nats_time", "consumer_delivered", "consumer_ack_floor"}})
		fmt.Println(string(encoded))
	case "producer":
		runProducer(*data, *prov, *speed, absRun)
	case "prep":
		runPrep(*wasl, *name, *threads, absRun)
	case "linker":
		runLinker(*wasl, *P, absRun)
	case "closure":
		runClosure(*wasl, *part, absRun)
	case "reviewer":
		runReviewer(*data, absRun, rcfg, *revLat, *hold, *seed)
	case "malaffi":
		runMalaffi(*data, *hieAddr, *speed, *seed, *fDup, *fReo, *fDrop, absRun, *pollSpeed, *lagFrac)
	case "fhirpoll":
		PollOverlap, ExportOff = *pollOverlap, os.Getenv("WASL_EXPORT_OFF") == "1"
		runFhirPoll("http://"+*hieAddr+"/fhir", absRun)
	case "hl7mllp":
		runHL7Listener(*hl7Addr, absRun)
	case "hl7send":
		runHL7Sender(*data, *hl7Addr, *seed, *fDup, *fReo, *hl7Drops, absRun)
	case "fhirsub":
		runFhirSub("http://"+*hieAddr+"/fhir", *hookAddr, *content, absRun)
	case "patient-link":
		token, err := provisionPatientLink(absRun, *patientCase, *patientTTL, time.Now().UTC())
		if err != nil {
			panic(err)
		}
		fmt.Println("/patient/access?link=" + token)
	case "serve":
		runServe(absRun, *data, *apiAddr)
	case "lungcache": // ask local Gemma 3 4B once per unique lung corpus text; write the replayable cache + reader scores
		runLungCache(*lungData, *readerURL, *out)
	case "drcache": // ask local Gemma 3 4B once per unique DR corpus text; write the replayable cache + reader scores
		runDRCache(*drData, *readerURL, *out)
	case "breastcache": // ask local Gemma 3 4B once per unique breast corpus text; write the replayable cache + reader scores
		runBreastCache(*breastData, *readerURL, *out)
	case "ldctcache": // ask local Gemma 3 4B once per unique LDCT corpus text; write the replayable cache + reader scores
		runLDCTCache(*ldctData, *readerURL, *out)
	case "ifhascache": // ask local Gemma 3 4B once per unique IFHAS follow-up note; write the replayable cache + reader scores
		runIfhasCache(*ifhasData, *readerURL, *out)
	case "readercache": // read every text of a report corpus + pathology templates with the live reader; write a gemma_cache.json-format file
		if LiveReader == nil || ReaderMode != "live" {
			panic("readercache needs -reader live")
		}
		n, ok := ReaderCache(*reports, *pathT, *out, 4)
		fmt.Printf("reader cache: %d texts, %d read OK -> %s\n", n, ok, *out)
	case "readerscore": // score regex + cached second reader against a keyed corpus (see readerscore.go)
		Judge = LoadJudge2(filepath.Join(*wasl, "gemma_cache.json"))
		b, _ := json.MarshalIndent(readerScore(*reports), "", " ")
		fmt.Println(string(b))
	case "readerwarm":
		if LiveReader == nil || ReaderMode != "live" {
			panic("readerwarm needs -reader live")
		}
		asked, ok := WarmReader(*data, 4)
		fmt.Printf("reader warm: %d unique report bodies, %d read OK, %d unavailable (will route to review)\n", asked, ok, asked-ok)
	case "orchestrate":
		SrcCfg = srcCfg{Source: *source, Content: *content, Hie: *hieAddr, Hook: *hookAddr, Dup: *fDup, Reorder: *fReo, Drop: *fDrop, SubKill: *subKill, HL7Addr: *hl7Addr, HL7Drops: *hl7Drops, PollSpeed: *pollSpeed, LagFrac: *lagFrac, Overlap: *pollOverlap}
		RevArgs = []string{"-revlat", fmt.Sprint(*revLat), "-seed", fmt.Sprint(*seed)}
		if *hold != "" {
			RevArgs = append(RevArgs, "-hold", *hold)
		}
		govArgs = []string{"-rerr", fmt.Sprint(rcfg.Err), "-rdouble", *rdouble, "-rseed", fmt.Sprint(*rseed), fmt.Sprintf("-dual=%v", *dual), "-ann", *annMode}
		govArgs = append(govArgs, v7Args...)
		govArgs = append(govArgs, h7Args...)
		if *annCB != "" {
			govArgs = append(govArgs, "-codebook", *annCB)
		}
		orchestrate(*data, *wasl, absRun, *P, *preps, *threads, *speed, *chaos, *natsKill, *replicas, *noReview, *seed)
	case "sim":
		simRun := ""
		if *run != "../run/default" {
			simRun = absRun
		}
		simOffline(*data, *wasl, *P, !*noReview, simRun)
	case "agentshadow": // standalone read-only shadow spike (agentshadow.go); never started by orchestrate
		os.Exit(agentShadowMain(*wasl, *data, absRun, *P, AgentShadowFlags{Mode: *agentMode, Items: *agentItems, Calls: *agentCalls,
			Deadline: *agentDeadline, Provider: *agentProvider, Idle: *agentIdle, Cache: *agentCache, Worker: *agentWorker, Python: *agentPython}))
	case "agentreplay":
		agentReplayMain(*data, *wasl, absRun, *P, rcfg, *agentRecsPath)
	case "audit":
		audit(*data, *wasl, absRun, *P)
	case "report7":
		report7(*data, *wasl, absRun, *P)
	case "demo":
		runDemo(fs.Args(), *data, absRun)
	case "sweep":
		if *norbac {
			RBACEnforce = false
			SweepErrs, SweepDoubles, SweepSeeds = []float64{0}, []string{"none"}, 1
		}
		if *quick {
			SweepErrs, SweepDoubles, SweepSeeds = []float64{0, 0.04}, []string{"none", "merge+close"}, 1
		}
		SweepRhos = nil
		for _, x := range strings.Split(*sweepRho, ",") {
			v, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				panic("bad -sweeprho " + x)
			}
			SweepRhos = append(SweepRhos, v)
		}
		sweep(*data, *wasl, *P, *sweepOut)
	case "ltt":
		lttCollect(*data, *wasl, *P, *lttGrid, rcfg, *lttOut)
	case "tamper":
		if err := tamper(*data, *wasl, absRun, *P, *role, *seed); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case "encembed":
		runEncEmbed(*encFile, *inFile, *out)
	case "encnoise":
		runEncNoise(*wasl, *split, *out, *seed)
	case "featdump":
		runFeatDump(*wasl, *encFile, *gbmFile, *split, *out, *persons, *seed)
	case "nameidf":
		runNameIDF(*wasl, *split, *out, *persons, parseIDFSeeds(*idfSeeds))
	case "annsynth":
		runAnnSynth(*wasl, *models, *out, *nRegs, *seed, *threads)
	case "annbench":
		c := BenchCfg{Mode: *mode, N: *nRegs, K: *kNN, NList: *nlist, NProbe: *nprobe, Rerank: *rerank, M: *hM, EfC: *efc, EfS: *efs,
			RecallN: *recallN, ExactSample: *exSample, SnapAt: *snapAt, Shuffle: *shuffle, QEvery: *qevery}
		res := runAnnBench(*out, c)
		b, _ := json.MarshalIndent(res, "", " ")
		fmt.Println(string(b))
	}
}

// ---------------- orchestrator ----------------

var RevArgs []string

type srcCfg struct {
	Source, Content, Hie, Hook  string
	PollSpeed, LagFrac, Overlap float64
	Dup, Reorder, Drop          float64
	SubKill                     int
	HL7Addr                     string
	HL7Drops                    int
}

var SrcCfg = srcCfg{Source: "file"}

type proc struct {
	name    string
	bin     string
	args    []string
	cmd     *exec.Cmd
	log     *os.File
	kills   int
	crashes int
	gen     int
	exited  <-chan struct{} // closed only after exec.Cmd.Wait has reaped this generation
	stopped bool            // staged finite producer shutdown also fences already-decided restarts
}

type Orch struct {
	run            string
	bin            string
	procs          map[string]*proc
	order          []string
	mu             sync.Mutex
	stopping       bool
	writersStopped bool
}

func (o *Orch) start(name string, bin string, args ...string) {
	o.mu.Lock()
	// Also guard the actual start: a supervisor may already have decided to
	// restart when snapshot shutdown acquired the lock.
	if o.stopping || (o.writersStopped && !strings.HasPrefix(name, "nats-")) {
		o.mu.Unlock()
		return
	}
	p, ok := o.procs[name]
	if !ok {
		p = &proc{name: name, args: args}
		o.procs[name] = p
		o.order = append(o.order, name)
		os.MkdirAll(filepath.Join(o.run, "logs"), 0700)
		p.log, _ = os.OpenFile(filepath.Join(o.run, "logs", name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	}
	if p.stopped {
		o.mu.Unlock()
		return
	}
	if bin != "" {
		p.bin = bin
	}
	if p.bin == "" {
		p.bin = o.bin
	}
	p.gen++
	gen := p.gen
	cmd := exec.Command(p.bin, p.args...)
	cmd.Stdout, cmd.Stderr = p.log, p.log
	cmd.Dir = filepath.Dir(o.bin)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	p.cmd = cmd
	exited := make(chan struct{})
	p.exited = exited
	o.mu.Unlock()
	fmt.Fprintf(p.log, "=== started pid %d at %s\n", cmd.Process.Pid, time.Now().Format(time.RFC3339Nano))
	// supervisor: restart on unexpected exit (restartPolicy: Always), unless killed by chaos or shutting down
	go func() {
		cmd.Wait()
		close(exited)
		time.Sleep(300 * time.Millisecond)
		o.mu.Lock()
		restart := !o.stopping && !o.writersStopped && !p.stopped && p.gen == gen && !strings.HasPrefix(name, "producer-") && name != "hl7send" // one-shot jobs exit when done
		if restart {
			p.crashes++
		}
		o.mu.Unlock()
		if restart {
			fmt.Fprintf(p.log, "=== supervisor: unexpected exit, restarting\n")
			o.start(name, "")
		}
	}()
}

func (o *Orch) kill(name string) {
	p := o.procs[name]
	o.mu.Lock()
	p.gen++ // chaos kill: supervisor must not restart; the chaos loop restarts it after the downtime
	o.mu.Unlock()
	p.cmd.Process.Signal(syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)
	p.kills++
	fmt.Fprintf(p.log, "=== SIGKILL at %s\n", time.Now().Format(time.RFC3339Nano))
}

func (o *Orch) stopAll() {
	o.mu.Lock()
	o.stopping = true
	o.mu.Unlock()
	for i := len(o.order) - 1; i >= 0; i-- {
		if p := o.procs[o.order[i]]; p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	time.Sleep(500 * time.Millisecond)
	for _, p := range o.procs {
		if p.cmd != nil && p.cmd.Process != nil {
			p.cmd.Process.Kill()
		}
	}
}

func readStatus(run, name string) map[string]interface{} {
	b, err := os.ReadFile(filepath.Join(run, "status-"+name+".json"))
	if err != nil {
		return nil
	}
	m := map[string]interface{}{}
	json.Unmarshal(b, &m)
	return m
}

func natsConf(run string, i int) string {
	dir := filepath.Join(run, fmt.Sprintf("nats-%d", i))
	os.MkdirAll(dir, 0700)
	os.Chmod(dir, 0700)
	routes := []string{}
	for j := 0; j < 3; j++ {
		routes = append(routes, fmt.Sprintf("nats-route://127.0.0.1:%d", 16822+PortOff+j))
	}
	conf := fmt.Sprintf(`server_name: n%d
listen: 127.0.0.1:%d
http: 127.0.0.1:%d
jetstream { store_dir: "%s", max_memory_store: 256MB, max_file_store: 4GB, cipher: chachapoly, key: $WASL_JS_KEY }
cluster { name: wsi, listen: 127.0.0.1:%d, routes: [%s] }
`, i, 14822+PortOff+i, 18822+PortOff+i, dir, 16822+PortOff+i, strings.Join(routes, ", "))
	p := filepath.Join(dir, "nats.conf")
	os.WriteFile(p, []byte(conf), 0600)
	os.Chmod(p, 0600)
	return p
}

func natsBinaryPath(executable string) string {
	return filepath.Join(filepath.Dir(executable), "nats-server")
}

func orchestrate(data, wasl, run string, P, preps, threads int, speed float64, chaos int, natsKill bool, replicas int, noReview bool, seed int64) {
	ensureMockCredentials()
	keepFiles := map[string][]byte{} // keep the live-reader ledgers across reruns (model answers are reused, not re-asked)
	for _, n := range []string{"reader_ledger.jsonl", "image_ledger.jsonl"} {
		if b, _ := os.ReadFile(filepath.Join(run, n)); len(b) > 0 {
			keepFiles[n] = b
		}
	}
	os.RemoveAll(run)
	os.MkdirAll(run, 0700)
	for n, b := range keepFiles {
		os.WriteFile(filepath.Join(run, n), b, 0600)
	}
	self, _ := os.Executable()
	o := &Orch{run: run, bin: self, procs: map[string]*proc{}}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; o.stopAll(); os.Exit(1) }()
	defer o.stopAll()
	// encryption at rest (ChaCha20-Poly1305) for every JetStream store, stream and KV. The key comes from WASL_JS_KEY (a
	// deployment keeps it in a KMS/HSM); the demo makes a random one per run and passes it only through the environment,
	// never into the run directory, so the on-disk stores are unreadable without it.
	// The nats-server config parser reads the substituted value as a token: one that starts with a digit is taken as a
	// number and the server refuses to start, so the key must start with a letter.
	if os.Getenv("WASL_JS_KEY") == "" {
		k := make([]byte, 32)
		crand.Read(k)
		os.Setenv("WASL_JS_KEY", "k"+hex.EncodeToString(k))
	}
	if c := os.Getenv("WASL_JS_KEY")[0]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
		panic("WASL_JS_KEY must start with a letter (nats-server parses a leading digit as a number)")
	}
	natsBin := natsBinaryPath(self)
	if NatsExternal { // deployment: the cluster runs outside the orchestrator (docker-compose nats-0..2)
		fmt.Printf("NATS: external cluster %s (not started or killed by the orchestrator)\n", NatsURL)
	} else {
		for i := 0; i < 3; i++ {
			o.start(fmt.Sprintf("nats-%d", i), natsBin, "-c", natsConf(run, i))
		}
		time.Sleep(1500 * time.Millisecond)
	}
	c := func(args ...string) []string {
		return append(append(args, "-data", data, "-wasl", wasl, "-run", run, "-P", fmt.Sprint(P)), govArgs...)
	}
	setup := exec.Command(self, c("setup", "-replicas", fmt.Sprint(replicas))...)
	setup.Stdout, setup.Stderr = os.Stdout, os.Stderr
	if err := setup.Run(); err != nil {
		panic(err)
	}
	o.start("linker", "", append(c("linker"), LinkFunnelArgs...)...)
	for k := 0; k < P; k++ {
		o.start(fmt.Sprintf("closure-%d", k), "", c("closure", "-part", fmt.Sprint(k))...)
	}
	for k := 0; k < preps; k++ {
		o.start(fmt.Sprintf("prep-%d", k), "", c("prep", "-name", fmt.Sprint(k), "-threads", fmt.Sprint(threads))...)
	}
	if !noReview {
		o.start("reviewer", "", append(c("reviewer"), RevArgs...)...)
	}
	time.Sleep(2500 * time.Millisecond) // encoder load in prep workers
	t0 := time.Now()
	if SrcCfg.Source == "fhir" {
		os.Setenv("WASL_MOCK_CALLBACK_URL", "http://"+SrcCfg.Hook+"/hook")
		o.start("malaffi", "", c("malaffi", "-speed", fmt.Sprint(speed), "-seed", fmt.Sprint(seed), "-hie", SrcCfg.Hie,
			"-fdup", fmt.Sprint(SrcCfg.Dup), "-freorder", fmt.Sprint(SrcCfg.Reorder), "-fdrop", fmt.Sprint(SrcCfg.Drop))...)
		time.Sleep(700 * time.Millisecond) // mock loads + sorts the cohort
		t0 = time.Now()
		o.start("fhirsub", "", c("fhirsub", "-hie", SrcCfg.Hie, "-hook", SrcCfg.Hook, "-content", SrcCfg.Content)...)
	} else if SrcCfg.Source == "fhirpoll" {
		ps := SrcCfg.PollSpeed
		if ps <= 0 {
			ps = 3 // simulated days per wall second: the 160-day feed in about a minute, like the subscription run
		}
		o.start("malaffi", "", c("malaffi", "-speed", fmt.Sprint(speed), "-seed", fmt.Sprint(seed), "-hie", SrcCfg.Hie,
			"-pollspeed", fmt.Sprint(ps), "-lagfrac", fmt.Sprint(SrcCfg.LagFrac))...)
		time.Sleep(700 * time.Millisecond)
		t0 = time.Now()
		o.start("fhirpoll", "", c("fhirpoll", "-hie", SrcCfg.Hie, "-overlap", fmt.Sprint(SrcCfg.Overlap))...)
	} else if SrcCfg.Source == "hl7" {
		o.start("hl7mllp", "", c("hl7mllp", "-hl7", SrcCfg.HL7Addr)...)
		time.Sleep(400 * time.Millisecond)
		t0 = time.Now()
		o.start("hl7send", "", c("hl7send", "-hl7", SrcCfg.HL7Addr, "-seed", fmt.Sprint(seed), "-fdup", fmt.Sprint(SrcCfg.Dup),
			"-freorder", fmt.Sprint(SrcCfg.Reorder), "-hl7drops", fmt.Sprint(SrcCfg.HL7Drops))...)
	} else {
		for _, pc := range Providers {
			o.start("producer-"+pc.ID, "", c("producer", "-prov", pc.ID, "-speed", fmt.Sprint(speed))...)
		}
	}
	// chaos schedule: kill/restart cycles spread over the expected ingest window
	rng := rand.New(rand.NewSource(seed))
	events := []map[string]interface{}{}
	expect := 15.0
	if speed > 0 {
		expect = TEnd / speed
	}
	if ChaosSpan > 0 { // larger cohorts: spread the kill/restart cycles over the longer feed (default 0 = unchanged)
		expect = ChaosSpan
	}
	victims := [][]string{{"linker"}, {fmt.Sprintf("closure-%d", rng.Intn(P)), "prep-0"}, {"linker", fmt.Sprintf("closure-%d", rng.Intn(P))},
		{fmt.Sprintf("closure-%d", rng.Intn(P)), "prep-1"}, {"linker", "prep-2"}}
	var subDone sync.WaitGroup
	subRng := rand.New(rand.NewSource(seed + 1))
	var evMu sync.Mutex
	subExpect := 6.0 // full-speed feed length (s) measured on this cohort
	if speed > 0 {
		subExpect = expect
	} else if ChaosSpan > 0 {
		subExpect = ChaosSpan * 6 / 15
	}
	if SrcCfg.Source == "hl7" && SrcCfg.SubKill > 0 { // kill/restart the MLLP listener mid-feed (sender reconnects, re-sends unACKed)
		subDone.Add(1)
		go func() {
			defer subDone.Done()
			for k := 0; k < SrcCfg.SubKill; k++ {
				at := subExpect * 3 * (float64(k) + 0.45) / float64(SrcCfg.SubKill)
				if d := time.Until(t0.Add(time.Duration(at * float64(time.Second)))); d > 0 {
					time.Sleep(d)
				}
				o.kill("hl7mllp")
				down := 1.5 + subRng.Float64()
				time.Sleep(time.Duration(down * float64(time.Second)))
				o.start("hl7mllp", "")
				evMu.Lock()
				events = append(events, map[string]interface{}{"cycle": fmt.Sprintf("hl7mllp-%d", k+1), "victims": []string{"hl7mllp"}, "t_kill": at, "down_s": down})
				evMu.Unlock()
				fmt.Printf("chaos: killed HL7 MLLP listener at %.1fs, restarted after %.1fs\n", at, down)
			}
		}()
	}
	if SrcCfg.Source == "fhirpoll" && SrcCfg.SubKill > 0 { // kill/restart the poller mid-feed (resumes from its KV watermark)
		subDone.Add(1)
		go func() {
			defer subDone.Done()
			for k := 0; k < SrcCfg.SubKill; k++ {
				at := 40.0 * (float64(k) + 0.45) / float64(SrcCfg.SubKill)
				if d := time.Until(t0.Add(time.Duration(at * float64(time.Second)))); d > 0 {
					time.Sleep(d)
				}
				o.kill("fhirpoll")
				down := 1.5 + subRng.Float64()
				time.Sleep(time.Duration(down * float64(time.Second)))
				o.start("fhirpoll", "")
				evMu.Lock()
				events = append(events, map[string]interface{}{"cycle": fmt.Sprintf("fhirpoll-%d", k+1), "victims": []string{"fhirpoll"}, "t_kill": at, "down_s": down})
				evMu.Unlock()
				fmt.Printf("chaos: killed FHIR poller at %.1fs, restarted after %.1fs\n", at, down)
			}
		}()
	}
	if SrcCfg.Source == "fhir" && SrcCfg.SubKill > 0 {
		subDone.Add(1)
		go func() {
			defer subDone.Done()
			for k := 0; k < SrcCfg.SubKill; k++ {
				at := subExpect * (float64(k) + 0.45) / float64(SrcCfg.SubKill)
				if d := time.Until(t0.Add(time.Duration(at * float64(time.Second)))); d > 0 {
					time.Sleep(d)
				}
				pre := readStatus(run, "fhirsub")
				o.kill("fhirsub")
				down := 1.5 + subRng.Float64()
				time.Sleep(time.Duration(down * float64(time.Second)))
				o.start("fhirsub", "")
				evMu.Lock()
				events = append(events, map[string]interface{}{"cycle": fmt.Sprintf("fhirsub-%d", k+1), "victims": []string{"fhirsub"},
					"t_kill": at, "down_s": down, "status_before_kill": map[string]interface{}{"fhirsub": pre}})
				evMu.Unlock()
				fmt.Printf("chaos: killed FHIR subscription consumer at %.1fs, restarted after %.1fs\n", at, down)
			}
		}()
	}
	done := make(chan bool)
	go func() {
		for cy := 0; cy < chaos; cy++ {
			at := expect * (float64(cy) + 0.5 + 0.3*rng.Float64()) / float64(chaos+1)
			if d := time.Until(t0.Add(time.Duration(at * float64(time.Second)))); d > 0 {
				time.Sleep(d)
			}
			vs := victims[cy%len(victims)]
			ev := map[string]interface{}{"cycle": cy + 1, "victims": vs, "t_kill": time.Since(t0).Seconds()}
			pre := map[string]interface{}{}
			for _, v := range vs {
				pre[v] = readStatus(run, v)
				o.kill(v)
			}
			down := 1.0 + rng.Float64()
			time.Sleep(time.Duration(down * float64(time.Second)))
			for _, v := range vs {
				o.start(v, "")
			}
			ev["down_s"], ev["status_before_kill"] = down, pre
			evMu.Lock()
			events = append(events, ev)
			evMu.Unlock()
			fmt.Printf("chaos cycle %d: killed %v at %.1fs, restarted after %.1fs\n", cy+1, vs, ev["t_kill"], down)
		}
		if natsKill && NatsExternal {
			fmt.Println("chaos: NATS node kill skipped (external cluster)")
		} else if natsKill {
			time.Sleep(time.Duration(expect * 0.1 * float64(time.Second)))
			o.kill("nats-2")
			ev := map[string]interface{}{"cycle": "nats", "victims": []string{"nats-2"}, "t_kill": time.Since(t0).Seconds()}
			time.Sleep(2 * time.Second)
			o.start("nats-2", natsBin)
			events = append(events, ev)
			fmt.Printf("chaos: killed NATS node n2 at %.1fs, restarted\n", ev["t_kill"])
		}
		subDone.Wait()
		done <- true
	}()
	// A timeout is an incomplete run, never permission to export an advancing
	// graph. The budget still starts at ingest start; it is not extended here.
	nc, js := connect("orchestrator")
	defer nc.Close()
	ctx, cancel := context.WithDeadline(context.Background(), t0.Add(QuietTimeout))
	defer cancel()
	sources := []string{}
	switch SrcCfg.Source {
	case "fhir":
		sources = append(sources, "fhirsub")
	case "fhirpoll":
		sources = append(sources, "fhirpoll")
	case "hl7":
		sources = append(sources, "hl7send")
	default:
		for _, pc := range Providers {
			sources = append(sources, "producer-"+pc.ID)
		}
	}
	chaosDone := false // includes subDone even when ordinary chaos is disabled
	var finite finiteMockShutdown
	var evidence snapshotFailureEvidence
	observe := func(ctx context.Context) (snapshotBoundary, error) {
		select {
		case <-done:
			chaosDone = true
		default:
		}
		var b snapshotBoundary
		var err error
		if !chaosDone {
			err = errors.New("chaos/source restart schedule still running")
		} else if SrcCfg.Source == "fhir" {
			err = finite.advance(ctx, js, run, chaosDone, o.stopFiniteMock)
		}
		if err == nil {
			b, err = captureSnapshotBoundary(ctx, js, run, P, noReview, sources, SrcCfg.Source == "fhir")
		}
		evidence.observe(b, err)
		return b, err
	}
	boundary, err := waitSnapshotBoundary(ctx, 500*time.Millisecond, 7, observe)
	if err != nil {
		writeStatus(run, "snapshot", map[string]any{"status": "incomplete", "error": err.Error()})
		emitSnapshotFailure(js, run, &evidence)
		panic(err)
	}
	tQuiet := time.Now()
	fi, oi := boundary.Streams["FHIR"], boundary.Streams["OUT"]
	lastOut := oi.LastTime
	wall := lastOut.Sub(t0).Seconds()
	throughput := 0.0
	if wall > 0 {
		throughput = float64(fi.Msgs) / wall
	}
	res := map[string]interface{}{"speed": speed, "P": P, "preps": preps, "threads": threads, "replicas": replicas,
		"fhir_msgs": fi.Msgs, "out_msgs": oi.Msgs, "t_start": t0, "t_last_out": lastOut, "t_quiet": tQuiet,
		"wall_s": wall, "throughput_ev_s": throughput, "source": SrcCfg}
	// Writer shutdown is awaited, not a deferred cancel or an assumed flush.
	// Any in-flight publication/status change during shutdown invalidates the
	// candidate cut. NATS stays available for the final checks and export.
	exportCtx, exportCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer exportCancel()
	stop := func(ctx context.Context) error {
		if err := o.stopSnapshotWriters(ctx); err != nil {
			return err
		}
		evMu.Lock()
		res["chaos"] = events
		evMu.Unlock()
		o.mu.Lock()
		sup := map[string]interface{}{}
		for n, p := range o.procs {
			sup[n] = map[string]int{"chaos_kills": p.kills, "unexpected_exits_restarted": p.crashes}
		}
		o.mu.Unlock()
		res["supervisor"] = sup
		res["linker"], res["reviewer"] = readStatus(run, "linker"), readStatus(run, "reviewer")
		res["fhirsub"], res["malaffi"] = readStatus(run, "fhirsub"), readStatus(run, "malaffi")
		cl := map[string]interface{}{}
		for k := 0; k < P; k++ {
			cl[fmt.Sprint(k)] = readStatus(run, fmt.Sprintf("closure-%d", k))
		}
		res["closures"] = cl
		return nil
	}
	if err := exportSnapshot(exportCtx, js, run, boundary, stop, observe, res); err != nil {
		emitSnapshotFailure(js, run, &evidence)
		panic(err)
	}
	fmt.Printf("quiescent: %d FHIR msgs, %d outputs, %.1fs, %.0f ev/s\n", fi.Msgs, oi.Msgs, wall, throughput)
}

var govArgs []string // v6 reviewer/governance flags forwarded to every child process

type LogMsg struct {
	Seq     uint64            `json:"seq"`
	Subject string            `json:"subj"`
	Hdr     map[string]string `json:"hdr"`
	Data    json.RawMessage   `json:"data"`
	Ts      int64             `json:"ts"`
}

// streamFence excludes consumer counts: export itself creates ordered consumers.
// Messages are immutable in JetStream, but counts/endpoints can change through
// append, retention or deletion; all of those changes invalidate a canonical cut.
type streamFence struct {
	Msgs     uint64    `json:"msgs"`
	First    uint64    `json:"first"`
	Last     uint64    `json:"last"`
	Bytes    uint64    `json:"bytes"`
	Deleted  int       `json:"deleted"`
	LastTime time.Time `json:"last_time"`
}

func (f streamFence) validate() error {
	if f.Deleted < 0 {
		return errors.New("negative deleted count")
	}
	if f.Msgs == 0 {
		if (f.First == 0 && f.Last == 0) || (f.First > 0 && f.First-1 == f.Last) {
			return nil
		}
		return errors.New("invalid empty stream endpoints")
	}
	if f.First == 0 || f.Last < f.First || f.Msgs > f.Last-f.First+1 {
		return errors.New("invalid stream count/endpoints")
	}
	return nil
}

func captureStreamFence(ctx context.Context, js jetstream.JetStream, name string) (streamFence, error) {
	s, err := js.Stream(ctx, name)
	if err != nil {
		return streamFence{}, fmt.Errorf("stream %s: %w", name, err)
	}
	i, err := s.Info(ctx)
	if err != nil {
		return streamFence{}, fmt.Errorf("info %s: %w", name, err)
	}
	if i == nil || i.Config.Name != name {
		return streamFence{}, fmt.Errorf("invalid info for %s", name)
	}
	f := streamFence{Msgs: i.State.Msgs, First: i.State.FirstSeq, Last: i.State.LastSeq, Bytes: i.State.Bytes, Deleted: i.State.NumDeleted, LastTime: i.State.LastTime}
	return f, f.validate()
}

func dumpStream(js jetstream.JetStream, stream, path string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f, err := captureStreamFence(ctx, js, stream)
	if err != nil {
		panic(err)
	}
	n, err := dumpStreamAt(ctx, js, stream, path, f)
	if err != nil {
		panic(err)
	}
	return n
}

// dumpStreamAt never resamples its fence. In particular, the final Fetch batch
// can contain later messages even when the outer count was below the endpoint.
// Sequence gaps are legal for retained streams: require the captured row count,
// first/last endpoints and strict order, not Last-First+1 rows.
func dumpStreamAt(ctx context.Context, js jetstream.JetStream, stream, path string, fence streamFence) (n int, err error) {
	if err = fence.validate(); err != nil {
		return 0, err
	}
	if err = ctx.Err(); err != nil {
		return 0, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-partial-*")
	if err != nil {
		return 0, err
	}
	tmp := f.Name()
	// Keep a failed partial under its private, noncanonical name as evidence.
	defer f.Close()
	var cons jetstream.Consumer
	if fence.Msgs > 0 {
		s, e := js.Stream(ctx, stream)
		if e != nil {
			return n, e
		}
		cons, e = s.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{})
		if e != nil {
			return n, e
		}
	}
	var last uint64
	for uint64(n) < fence.Msgs {
		if e := ctx.Err(); e != nil {
			return n, e
		}
		want := fence.Msgs - uint64(n)
		if want > 1000 {
			want = 1000
		}
		batch, e := cons.Fetch(int(want), jetstream.FetchMaxWait(2*time.Second))
		if e != nil {
			return n, fmt.Errorf("dump %s fetch: %w", stream, e)
		}
		if batch == nil {
			return n, fmt.Errorf("dump %s: nil batch", stream)
		}
		got := 0
		messages := batch.Messages()
	batchLoop:
		for {
			select {
			case <-ctx.Done():
				return n, ctx.Err()
			case m, ok := <-messages:
				if !ok {
					break batchLoop
				}
				if m == nil {
					return n, fmt.Errorf("dump %s: nil message", stream)
				}
				md, e := m.Metadata()
				if e != nil || md == nil {
					return n, fmt.Errorf("dump %s: invalid message metadata (%v)", stream, e)
				}
				seq := md.Sequence.Stream
				if md.Stream != stream || seq == 0 || md.Timestamp.IsZero() || seq <= last {
					return n, fmt.Errorf("dump %s: invalid sequence/stream/timestamp at %d", stream, seq)
				}
				// Later appends may be delivered in this same batch, but never
				// written or counted as a substitute for a missing captured row.
				if seq > fence.Last {
					if uint64(n) != fence.Msgs {
						return n, fmt.Errorf("dump %s incomplete before endpoint %d", stream, fence.Last)
					}
					continue
				}
				if uint64(n) >= fence.Msgs || (n == 0 && seq != fence.First) {
					return n, fmt.Errorf("dump %s: captured count/first endpoint mismatch", stream)
				}
				h := map[string]string{}
				for k, v := range m.Headers() {
					if len(v) == 0 {
						return n, fmt.Errorf("dump %s: empty header %s", stream, k)
					}
					h[k] = v[0]
				}
				data := m.Data()
				if !json.Valid(data) {
					data, _ = json.Marshal(string(data))
				} // preserve existing non-JSON log representation
				b, e := json.Marshal(LogMsg{Seq: seq, Subject: m.Subject(), Hdr: h, Data: data, Ts: md.Timestamp.UnixNano()})
				if e != nil {
					return n, e
				}
				if _, e = f.Write(append(b, '\n')); e != nil {
					return n, e
				}
				last = seq
				n++
				got++
			}
		}
		if e := batch.Error(); e != nil {
			return n, fmt.Errorf("dump %s batch: %w", stream, e)
		}
		if got == 0 {
			return n, fmt.Errorf("dump %s incomplete: %d of %d", stream, n, fence.Msgs)
		}
	}
	if fence.Msgs > 0 && last != fence.Last {
		return n, fmt.Errorf("dump %s: last endpoint mismatch", stream)
	}
	if err = f.Sync(); err != nil {
		return n, err
	}
	if err = f.Close(); err != nil {
		return n, err
	}
	err = os.Rename(tmp, path)
	return n, err
}

type snapshotClosure struct {
	Next uint64 `json:"next_pseq"`
	OSeq uint64 `json:"ledger_oseq"`
	Head string `json:"ledger_head"`
}
type snapshotAck struct {
	Last     uint64 `json:"ack_floor"`
	Pending  uint64 `json:"pending"`
	Inflight int    `json:"inflight"`
}
type snapshotBoundary struct {
	Streams      map[string]streamFence `json:"streams"`
	Processed    uint64                 `json:"processed"`
	PSeq         []uint64               `json:"pseq"`
	Closures     []snapshotClosure      `json:"closures"`
	ReviewerLast uint64                 `json:"reviewer_last"`
	Consumers    map[string]snapshotAck `json:"consumers"`
}

// Status files are observations, not authority to coerce missing/null/negative
// or fractional fields into zero (or to panic on failed type assertions).
func snapshotStatus(run, name string) (map[string]json.RawMessage, error) {
	b, err := os.ReadFile(filepath.Join(run, "status-"+name+".json"))
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateKeys(b); err != nil {
		return nil, err
	}
	var st map[string]json.RawMessage
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st == nil {
		return nil, fmt.Errorf("missing status %s", name)
	}
	for _, key := range []string{"error", "link_funnel_error"} {
		if raw, ok := st[key]; ok && string(raw) != `""` {
			return nil, fmt.Errorf("%s status %s: %s", name, key, raw)
		}
	}
	return st, nil
}
func snapshotField(st map[string]json.RawMessage, key string, dest any) error {
	raw, ok := st[key]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("missing status field %s", key)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("status field %s: %w", key, err)
	}
	return nil
}
func snapshotUint(st map[string]json.RawMessage, key string) (uint64, error) {
	var n uint64
	err := snapshotField(st, key, &n)
	return n, err
}
func snapshotConsumerAck(ctx context.Context, js jetstream.JetStream, stream, name string, last uint64) (snapshotAck, error) {
	c, err := js.Consumer(ctx, stream, name)
	if err != nil {
		return snapshotAck{}, err
	}
	i, err := c.Info(ctx)
	if err != nil {
		return snapshotAck{}, err
	}
	if i == nil || i.Stream != stream || i.Name != name || i.Config.AckPolicy != jetstream.AckExplicitPolicy ||
		i.Config.DeliverPolicy != jetstream.DeliverAllPolicy || i.Config.FilterSubject != "" || len(i.Config.FilterSubjects) != 0 || i.NumRedelivered != 0 {
		return snapshotAck{}, fmt.Errorf("invalid/undrained %s/%s consumer info", stream, name)
	}
	a := snapshotAck{Last: i.AckFloor.Stream, Pending: i.NumPending, Inflight: i.NumAckPending}
	if a.Pending != 0 || a.Inflight != 0 || a.Last != last {
		return a, fmt.Errorf("%s/%s undrained: pending=%d inflight=%d ack=%d want=%d", stream, name, a.Pending, a.Inflight, a.Last, last)
	}
	return a, nil
}

func captureSnapshotBoundary(ctx context.Context, js jetstream.JetStream, run string, P int, noReview bool, sources []string, inbox bool) (b snapshotBoundary, err error) {
	if err = ctx.Err(); err != nil {
		return b, err
	}
	if P <= 0 || len(sources) == 0 {
		return b, errors.New("missing snapshot partitions/sources")
	}
	for _, name := range sources {
		st, e := snapshotStatus(run, name)
		if e != nil {
			return b, e
		}
		var done bool
		if e = snapshotField(st, "done", &done); e != nil {
			return b, e
		}
		if !done {
			return b, fmt.Errorf("source %s unfinished", name)
		}
	}
	b.Streams = map[string]streamFence{}
	b.Consumers = map[string]snapshotAck{}
	for _, name := range []string{"FHIR", "PREP", "FACTS", "OUT"} {
		f, e := captureStreamFence(ctx, js, name)
		if e != nil {
			return b, e
		}
		// A retained prefix with holes cannot represent a newly completed
		// canonical run, even though the individual dumper supports gaps.
		if f.Msgs != f.Last || (f.Msgs > 0 && f.First != 1) || f.Deleted != 0 {
			return b, fmt.Errorf("%s canonical history incomplete", name)
		}
		b.Streams[name] = f
	}
	fhir, prep := b.Streams["FHIR"], b.Streams["PREP"]
	if prep.Msgs != fhir.Msgs {
		return b, errors.New("FHIR/PREP not drained")
	}
	ls, e := snapshotStatus(run, "linker")
	if e != nil {
		return b, e
	}
	if b.Processed, e = snapshotUint(ls, "processed"); e != nil {
		return b, e
	}
	inflight, e := snapshotUint(ls, "inflight")
	if e != nil {
		return b, e
	}
	if b.Processed != fhir.Last || inflight != 0 {
		return b, errors.New("linker unfinished/inflight")
	}
	if e = snapshotField(ls, "pseq", &b.PSeq); e != nil {
		return b, e
	}
	if len(b.PSeq) != P {
		return b, errors.New("invalid linker partition watermarks")
	}
	var facts, outs uint64
	for k := 0; k < P; k++ {
		st, e := snapshotStatus(run, fmt.Sprintf("closure-%d", k))
		if e != nil {
			return b, e
		}
		var c snapshotClosure
		if c.Next, e = snapshotUint(st, "next_pseq"); e != nil {
			return b, e
		}
		if c.OSeq, e = snapshotUint(st, "ledger_oseq"); e != nil {
			return b, e
		}
		if e = snapshotField(st, "ledger_head", &c.Head); e != nil {
			return b, e
		}
		inflight, e := snapshotUint(st, "inflight")
		if e != nil {
			return b, e
		}
		if c.Next == 0 || c.Next-1 != b.PSeq[k] || inflight != 0 {
			return b, fmt.Errorf("closure %d unfinished/inflight", k)
		}
		if c.OSeq > 0 && len(c.Head) != 64 {
			return b, fmt.Errorf("closure %d invalid ledger anchor", k)
		}
		if ^uint64(0)-facts < b.PSeq[k] || ^uint64(0)-outs < c.OSeq {
			return b, errors.New("snapshot count overflow")
		}
		facts += b.PSeq[k]
		outs += c.OSeq
		b.Closures = append(b.Closures, c)
	}
	if facts != b.Streams["FACTS"].Msgs || outs != b.Streams["OUT"].Msgs {
		return b, errors.New("FACTS/OUT publications not drained")
	}
	a, e := snapshotConsumerAck(ctx, js, "FHIR", "prep", fhir.Last)
	if e != nil {
		return b, e
	}
	b.Consumers["FHIR/prep"] = a
	if !noReview {
		st, e := snapshotStatus(run, "reviewer")
		if e != nil {
			return b, e
		}
		if b.ReviewerLast, e = snapshotUint(st, "last_out_seq"); e != nil {
			return b, e
		}
		if b.ReviewerLast != b.Streams["OUT"].Last {
			return b, errors.New("reviewer unfinished")
		}
		// LastOut alone can pass despite an earlier failed review: require the
		// durable explicit-ACK floor and no pending delivery/effect as well.
		a, e := snapshotConsumerAck(ctx, js, "OUT", "reviewer", b.Streams["OUT"].Last)
		if e != nil {
			return b, e
		}
		b.Consumers["OUT/reviewer"] = a
	}
	if inbox {
		f, e := captureStreamFence(ctx, js, "FHIRIN")
		if e != nil {
			return b, e
		}
		b.Streams["FHIRIN"] = f
		if f.Msgs != 0 {
			return b, errors.New("notification inbox unfinished")
		}
		a, e := snapshotConsumerAck(ctx, js, "FHIRIN", "notifications", f.Last)
		if e != nil {
			return b, e
		}
		b.Consumers["FHIRIN/notifications"] = a
	}
	return b, ctx.Err()
}

func waitSnapshotBoundary(ctx context.Context, interval time.Duration, samples int, observe func(context.Context) (snapshotBoundary, error)) (snapshotBoundary, error) {
	if interval <= 0 || samples < 2 {
		return snapshotBoundary{}, errors.New("snapshot incomplete: invalid stability window")
	}
	var previous []byte
	stable := 0
	lastErr := errors.New("no drained observation")
	for {
		if err := ctx.Err(); err != nil {
			return snapshotBoundary{}, fmt.Errorf("snapshot incomplete: bounded quiescence wait: %w (last observation: %v)", err, lastErr)
		}
		b, err := observe(ctx)
		if ctx.Err() != nil {
			continue
		}
		if err != nil {
			stable = 0
			previous = nil
			lastErr = err
		} else {
			next := canon(b)
			if bytes.Equal(previous, next) {
				stable++
			} else {
				stable = 1
				previous = next
			}
			if stable >= samples {
				return b, nil
			}
			lastErr = errors.New("boundary not yet stable")
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
}

// Failure evidence is aggregate-only and bounded. Never serialize raw errors,
// status maps, ledger heads, KV records, source bodies or worker configuration.
// The container console survives volume cleanup; the same private artifact is
// useful in modes which retain the run directory. Neither is a success export.
type snapshotFailureEvidence struct {
	LastGood    map[string]any   `json:"last_good"`
	LastError   map[string]any   `json:"last_error"`
	Differences []map[string]any `json:"first_differences"`
	previous    map[string]any
}

func snapshotDiagnosticBoundary(b snapshotBoundary, err error) map[string]any {
	v := map[string]any{}
	for _, name := range []string{"FHIR", "PREP", "FACTS", "OUT", "FHIRIN"} {
		if f, ok := b.Streams[name]; ok {
			for key, value := range map[string]any{"msgs": f.Msgs, "first": f.First, "last": f.Last, "bytes": f.Bytes, "deleted": f.Deleted, "last_time": f.LastTime} {
				v["streams."+name+"."+key] = value
			}
		}
	}
	for _, name := range []string{"FHIR/prep", "OUT/reviewer", "FHIRIN/notifications"} {
		if a, ok := b.Consumers[name]; ok {
			v["consumers."+name+".ack_floor"], v["consumers."+name+".pending"], v["consumers."+name+".inflight"] = a.Last, a.Pending, a.Inflight
		}
	}
	if b.PSeq != nil {
		v["processed"] = b.Processed
		for i, n := range b.PSeq {
			if i >= 16 {
				break
			}
			v[fmt.Sprintf("pseq.%d", i)] = n
		}
	}
	for i, c := range b.Closures {
		if i >= 16 {
			break
		}
		v[fmt.Sprintf("closures.%d.next_pseq", i)] = c.Next
		v[fmt.Sprintf("closures.%d.ledger_oseq", i)] = c.OSeq
	}
	_, reviewed := b.Consumers["OUT/reviewer"]
	if reviewed || (err != nil && err.Error() == "reviewer unfinished") {
		v["reviewer_last"] = b.ReviewerLast
	}
	return v
}

func snapshotDiagnosticReason(err error) string {
	if err == nil {
		return "drained_observation"
	}
	// Exact allow-list only: wrapping or echoing arbitrary transport/status
	// errors can disclose endpoints, credentials, resource identities or paths.
	for _, safe := range []string{"reviewer unfinished", "notification inbox unfinished", "FHIR/PREP not drained", "linker unfinished/inflight", "FACTS/OUT publications not drained", "chaos/source restart schedule still running", "finite mock feed unfinished", "finite subscription unfinished", "finite subscription source coverage unfinished", "finite subscription durable pending unavailable/unfinished", "finite subscription durable completion unavailable", "finite notification inbox unfinished"} {
		if err.Error() == safe {
			return safe
		}
	}
	return "observation_failed_details_not_published"
}

func (e *snapshotFailureEvidence) observe(b snapshotBoundary, err error) {
	v := snapshotDiagnosticBoundary(b, err)
	if e.previous != nil {
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var diffs []map[string]any
		for _, key := range keys {
			if before, ok := e.previous[key]; ok && !bytes.Equal(canon(before), canon(v[key])) {
				diffs = append(diffs, map[string]any{"field": key, "before": before, "after": v[key]})
				if len(diffs) == 8 {
					break
				}
			}
		}
		if len(diffs) > 0 {
			e.Differences = diffs
		}
	}
	e.previous = v
	observation := map[string]any{"observed_at": time.Now().UTC(), "reason": snapshotDiagnosticReason(err), "captured_fields": v, "uncaptured_fields": "unavailable"}
	if err == nil {
		e.LastGood = observation
	} else {
		e.LastError = observation
	}
}

func snapshotOperationalStatus(run, name string, fields ...string) map[string]any {
	result := map[string]any{}
	for _, key := range fields {
		result[key] = "unavailable"
	}
	f, err := os.Open(filepath.Join(run, "status-"+name+".json"))
	if err != nil {
		return result
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	var st map[string]json.RawMessage
	if err != nil || len(b) > 1<<20 || rejectDuplicateKeys(b) != nil || json.Unmarshal(b, &st) != nil {
		return result
	}
	for _, key := range fields {
		selected, field := st, key
		if parent, child, nested := strings.Cut(key, "."); nested {
			field, selected = child, nil
			if json.Unmarshal(st[parent], &selected) != nil {
				continue
			}
		}
		if n, err := snapshotUint(selected, field); err == nil {
			result[key] = n
		} else {
			var flag bool
			if snapshotField(selected, field, &flag) == nil {
				result[key] = flag
			}
		}
	}
	return result
}

func snapshotFailureReport(ctx context.Context, js jetstream.JetStream, run string, evidence *snapshotFailureEvidence) map[string]any {
	history := map[string]any{"last_good": "unavailable", "last_error": "unavailable", "first_differences": "unavailable_no_changed_samples"}
	if evidence.LastGood != nil {
		history["last_good"] = evidence.LastGood
	}
	if evidence.LastError != nil {
		history["last_error"] = evidence.LastError
	}
	if len(evidence.Differences) > 0 {
		history["first_differences"] = evidence.Differences
	}
	report := map[string]any{"event": "snapshot_incomplete_diagnostics", "evidence": history, "reviewer_outbox_counts": "unavailable_not_exposed", "durable_reviewer_cursor": "unavailable"}
	streams := map[string]any{}
	for _, name := range []string{"OUT", "FHIRIN"} {
		streams[name] = "unavailable"
		if f, err := captureStreamFence(ctx, js, name); err == nil {
			streams[name] = f
		}
	}
	report["streams"] = streams
	consumers := map[string]any{}
	for _, pair := range [][2]string{{"FHIR", "prep"}, {"OUT", "reviewer"}, {"FHIRIN", "notifications"}} {
		key := pair[0] + "/" + pair[1]
		consumers[key] = "unavailable"
		c, err := js.Consumer(ctx, pair[0], pair[1])
		if err != nil {
			continue
		}
		i, err := c.Info(ctx)
		if err == nil && i != nil && i.Stream == pair[0] && i.Name == pair[1] {
			consumers[key] = map[string]any{"ack_floor_stream": i.AckFloor.Stream, "ack_floor_consumer": i.AckFloor.Consumer, "delivered_stream": i.Delivered.Stream, "delivered_consumer": i.Delivered.Consumer, "pending": i.NumPending, "ack_pending": i.NumAckPending, "redelivered": i.NumRedelivered}
		}
	}
	report["consumers"] = consumers
	report["sampled_reviewer"] = snapshotOperationalStatus(run, "reviewer", "last_out_seq", "answered", "wrong", "deduped_second_id", "deduped_link")
	report["sampled_linker"] = snapshotOperationalStatus(run, "linker", "processed", "inflight", "pub_retries")
	report["sampled_subscription"] = snapshotOperationalStatus(run, "fhirsub", "ack_hwm", "max_seen", "next", "buffered", "done", "feed_off", "eof_sent", "stats.durable_pending")
	report["sampled_mock"] = snapshotOperationalStatus(run, "malaffi", "events", "released", "stats.posted_heartbeat")
	if kv, err := js.KeyValue(ctx, "REVIEWER"); err == nil {
		if entry, err := kv.Get(ctx, reviewerStatusKey); err == nil && entry != nil {
			var st map[string]json.RawMessage
			if len(entry.Value()) <= 4096 && rejectDuplicateKeys(entry.Value()) == nil && json.Unmarshal(entry.Value(), &st) == nil {
				if n, err := snapshotUint(st, "last_out_seq"); err == nil {
					report["durable_reviewer_cursor"] = n
				}
			}
		}
	}
	return report
}

func emitSnapshotFailure(js jetstream.JetStream, run string, evidence *snapshotFailureEvidence) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	report := snapshotFailureReport(ctx, js, run, evidence)
	// Emit before any panic/deferred process teardown, even if artifact I/O fails.
	b, _ := json.Marshal(report)
	fmt.Fprintln(os.Stderr, string(b))
	writeStatus(run, "snapshot-failure", report)
}

// Off-feed heartbeats are still real FHIRIN publications. Stop only the finite
// mock before the full equality window; the subscription and feedback graph stay
// alive. A stop failure is sticky: it cannot be retried into a successful cut.
type finiteMockShutdown struct {
	stopped bool
	stopErr error
}

func (s *finiteMockShutdown) advance(ctx context.Context, js jetstream.JetStream, run string, chaosDone bool, stop func(context.Context) error) error {
	if !chaosDone {
		return errors.New("chaos/source restart schedule still running")
	}
	if s.stopErr != nil {
		return s.stopErr
	}
	if s.stopped {
		return nil
	}
	if err := completedMockSource(ctx, js, run); err != nil {
		return err
	}
	if s.stopErr = stop(ctx); s.stopErr != nil {
		return s.stopErr
	}
	s.stopped = true
	return nil
}

func completedMockSource(ctx context.Context, js jetstream.JetStream, run string) error {
	m, err := snapshotStatus(run, "malaffi")
	if err != nil {
		return err
	}
	var status string
	if err := snapshotField(m, "status", &status); err != nil {
		return err
	}
	total, err := snapshotUint(m, "events")
	if err != nil {
		return err
	}
	released, err := snapshotUint(m, "released")
	if err != nil {
		return err
	}
	if status != "off" || total == 0 || total >= 1<<63-1 || released != total {
		return errors.New("finite mock feed unfinished")
	}
	st, err := snapshotStatus(run, "fhirsub")
	if err != nil {
		return err
	}
	for _, key := range []string{"done", "feed_off", "eof_sent"} {
		var complete bool
		if err := snapshotField(st, key, &complete); err != nil {
			return err
		}
		if !complete {
			return errors.New("finite subscription unfinished")
		}
	}
	var sub string
	if err := snapshotField(st, "sub", &sub); err != nil || sub == "" {
		return errors.New("finite subscription identity unavailable")
	}
	var subscriptions map[string]map[string]json.RawMessage
	if err := snapshotField(st, "subscriptions", &subscriptions); err != nil {
		return err
	}
	ref := canonicalSubscriptionRef(sub)
	// The local mock owns exactly one finite subscription. Unknown additional
	// sources are not authority to stop their resource server.
	if len(subscriptions) != 1 || subscriptions[ref] == nil {
		return errors.New("finite subscription coverage unavailable")
	}
	for _, state := range []map[string]json.RawMessage{st, subscriptions[ref]} {
		for key, want := range map[string]uint64{"ack_hwm": total, "max_seen": total, "next": total + 1, "buffered": 0} {
			got, err := snapshotUint(state, key)
			if err != nil {
				return err
			}
			if got != want {
				return errors.New("finite subscription source coverage unfinished")
			}
		}
		for _, key := range []string{"feed_off", "eof_sent"} {
			var complete bool
			if err := snapshotField(state, key, &complete); err != nil || !complete {
				return errors.New("finite subscription EOF unavailable")
			}
		}
	}
	var stats map[string]json.RawMessage
	if err := snapshotField(st, "stats", &stats); err != nil {
		return err
	}
	for _, state := range []map[string]json.RawMessage{stats, subscriptions[ref]} {
		pending, err := snapshotUint(state, "durable_pending")
		if err != nil || pending != 0 {
			return errors.New("finite subscription durable pending unavailable/unfinished")
		}
	}
	// EOF in a sampled file alone is insufficient: a subscription restart must
	// recover the same completed prefix without needing the mock's HTTP server.
	kv, err := js.KeyValue(ctx, "FHIRSUB")
	if err != nil {
		return err
	}
	entry, err := kv.Get(ctx, checkpointKey(ref))
	if err != nil {
		return err
	}
	var cp subscriptionCheckpoint
	if entry == nil || rejectDuplicateKeys(entry.Value()) != nil || json.Unmarshal(entry.Value(), &cp) != nil ||
		canonicalSubscriptionRef(cp.Sub) != ref || cp.HWM != int64(total) || !cp.Eof {
		return errors.New("finite subscription durable completion unavailable")
	}
	f, err := captureStreamFence(ctx, js, "FHIRIN")
	if err != nil {
		return err
	}
	if f.Msgs != 0 {
		return errors.New("finite notification inbox unfinished")
	}
	if _, err := snapshotConsumerAck(ctx, js, "FHIRIN", "notifications", f.Last); err != nil {
		return err
	}
	// An accepted id-only notification's decode/HTTP fetch precedes its ACK.
	// The drained inbox excludes those active fetches. A notification accepted
	// just after this observation can only contain off-feed heartbeats or events
	// <= total: decodeContext skips their vreads because next == total+1, also
	// after restart from the verified checkpoint. Its publication/ACK may still
	// advance FHIRIN; the unchanged full window is deliberately AFTER mock exit.
	return ctx.Err()
}

func (o *Orch) stopFiniteMock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	p := o.procs["malaffi"]
	if p == nil || p.cmd == nil || p.cmd.Process == nil || p.exited == nil {
		o.mu.Unlock()
		return errors.New("finite mock missing process/exit acknowledgement")
	}
	p.stopped = true
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		o.mu.Unlock()
		return fmt.Errorf("finite mock stop: %w", err)
	}
	exited := p.exited
	o.mu.Unlock()
	select {
	case <-exited:
		return ctx.Err()
	case <-ctx.Done():
		return fmt.Errorf("finite mock exit not acknowledged: %w", ctx.Err())
	}
}

// No worker implements a graceful flush protocol. First prove it drained;
// then prevent restarts, signal, and await actual process exit. Finally the
// caller rechecks every stream, ACK floor and ledger watermark. A change or
// timeout is failure, never a claim that SIGTERM itself flushed anything.
func (o *Orch) stopSnapshotWriters(ctx context.Context) error {
	o.mu.Lock()
	o.writersStopped = true
	var writers []*proc
	for _, name := range o.order {
		if !strings.HasPrefix(name, "nats-") {
			writers = append(writers, o.procs[name])
		}
	}
	for _, p := range writers {
		if p.cmd != nil && p.cmd.Process != nil {
			if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
				o.mu.Unlock()
				return err
			}
		}
	}
	o.mu.Unlock()
	for _, p := range writers {
		if p.exited == nil {
			return fmt.Errorf("snapshot incomplete: writer %s has no exit acknowledgement", p.name)
		}
		select {
		case <-p.exited:
		case <-ctx.Done():
			for _, p := range writers {
				if p.cmd != nil && p.cmd.Process != nil {
					p.cmd.Process.Kill()
				}
			}
			return fmt.Errorf("snapshot incomplete: waiting for writer %s exit: %w", p.name, ctx.Err())
		}
	}
	return ctx.Err()
}

// Read staged logs strictly, without normalizing, reconstructing or rehashing
// the original producer rows. Only recompute/compare the existing anchor rules.
func scanSnapshotLog(path string, visit func(LogMsg) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 65536), 1<<24)
	for s.Scan() {
		var m LogMsg
		if err := json.Unmarshal(s.Bytes(), &m); err != nil {
			return err
		}
		if err := visit(m); err != nil {
			return err
		}
	}
	return s.Err()
}
func snapshotAnchors(stage string, b snapshotBoundary) (map[string]string, error) {
	anchors := map[string]string{"fhir": ""}
	if err := scanSnapshotLog(filepath.Join(stage, "fhir_log.ndjson"), func(m LogMsg) error {
		anchors["fhir"] = sha(append([]byte(anchors["fhir"]+"|"+utoa(m.Seq)+"|"+m.Subject+"|"+m.Hdr["Nats-Msg-Id"]+"|"), m.Data...))
		return nil
	}); err != nil {
		return nil, err
	}
	seenPrep := map[uint64]bool{}
	if err := scanSnapshotLog(filepath.Join(stage, "prep_log.ndjson"), func(m LogMsg) error {
		var p Prep
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return err
		}
		if p.Seq == 0 || p.Seq > b.Streams["FHIR"].Last || seenPrep[p.Seq] {
			return errors.New("PREP missing/duplicate/source outside snapshot")
		}
		seenPrep[p.Seq] = true
		return nil
	}); err != nil {
		return nil, err
	}
	if uint64(len(seenPrep)) != b.Streams["FHIR"].Msgs {
		return nil, errors.New("PREP source coverage incomplete")
	}
	facts := make([]map[uint64]bool, len(b.Closures))
	for k := range facts {
		facts[k] = map[uint64]bool{}
	}
	if err := scanSnapshotLog(filepath.Join(stage, "facts_log.ndjson"), func(m LogMsg) error {
		var f FactsMsg
		if err := json.Unmarshal(m.Data, &f); err != nil {
			return err
		}
		if f.Part < 0 || f.Part >= len(facts) || m.Subject != fmt.Sprintf("facts.p%d", f.Part) || f.InSeq == 0 || f.InSeq > b.Streams["FHIR"].Last {
			return errors.New("FACTS outside source/partition boundary")
		}
		if f.PSeq == 0 || f.PSeq > b.PSeq[f.Part] || facts[f.Part][f.PSeq] {
			return errors.New("FACTS duplicate/outside partition watermark")
		}
		facts[f.Part][f.PSeq] = true
		return nil
	}); err != nil {
		return nil, err
	}
	type link struct{ prev, hash string }
	ledgers := make([]map[uint64]link, len(b.Closures))
	for k := range ledgers {
		ledgers[k] = map[uint64]link{}
	}
	if err := scanSnapshotLog(filepath.Join(stage, "out_log.ndjson"), func(m LogMsg) error {
		var env OutEnvelope
		if err := json.Unmarshal(m.Data, &env); err != nil {
			return err
		}
		k := -1
		for part := range ledgers {
			if m.Subject == fmt.Sprintf("out.p%d", part) {
				k = part
				break
			}
		}
		if k < 0 || env.OSeq == 0 || env.OSeq > b.Closures[k].OSeq {
			return errors.New("OUT outside partition ledger boundary")
		}
		if _, ok := ledgers[k][env.OSeq]; ok {
			return errors.New("duplicate OUT ledger sequence")
		}
		if env.PSeq == 0 || env.PSeq > b.PSeq[k] || env.FSeq == 0 || env.FSeq > b.Streams["FACTS"].Last {
			return errors.New("OUT outside FACTS boundary")
		}
		var id string
		var body []byte
		var inSeq uint64
		switch {
		case env.Kind == "decision" && env.Dec != nil && env.Item == nil:
			d := env.Dec
			id, body, inSeq = d.ID, canon(d), d.InSeq
			if d.Sig != decSig(d) || !verifyChain(d.Chain) {
				return errors.New("OUT original decision signature/chain invalid")
			}
		case env.Kind == "review" && env.Item != nil && env.Dec == nil:
			r := env.Item
			id, body, inSeq = r.ItemID, canon(r), r.InSeq
			if !verifyChain(r.Chain) {
				return errors.New("OUT original review chain invalid")
			}
		default:
			return errors.New("unknown OUT envelope")
		}
		if id == "" || inSeq == 0 || inSeq > b.Streams["FHIR"].Last || env.Hash != ledgerHash(env.Prev, env.OSeq, id, env.Kind, body) {
			return errors.New("OUT original ledger hash/source invalid")
		}
		ledgers[k][env.OSeq] = link{env.Prev, env.Hash}
		return nil
	}); err != nil {
		return nil, err
	}
	for k, c := range b.Closures {
		if uint64(len(facts[k])) != b.PSeq[k] || uint64(len(ledgers[k])) != c.OSeq {
			return nil, errors.New("partition counts incomplete")
		}
		head := ""
		for seq := uint64(1); seq <= c.OSeq; seq++ {
			l, ok := ledgers[k][seq]
			if !ok || l.prev != head {
				return nil, errors.New("OUT original ledger chain incomplete")
			}
			head = l.hash
		}
		if head != c.Head {
			return nil, fmt.Errorf("out.p%d original ledger anchor mismatch", k)
		}
		anchors[fmt.Sprintf("out.p%d", k)] = head
	}
	return anchors, nil
}

func exportSnapshot(ctx context.Context, js jetstream.JetStream, run string, b snapshotBoundary, stop func(context.Context) error, observe func(context.Context) (snapshotBoundary, error), res map[string]any) (err error) {
	defer func() {
		if err != nil {
			writeStatus(run, "snapshot", map[string]any{"status": "incomplete", "error": err.Error(), "candidate": b})
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if stop == nil {
		return errors.New("snapshot incomplete: no awaited writer shutdown")
	}
	if err = stop(ctx); err != nil {
		return err
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		now, err := observe(ctx)
		if err != nil {
			return fmt.Errorf("snapshot incomplete: %w", err)
		}
		if !bytes.Equal(canon(b), canon(now)) {
			return errors.New("snapshot incomplete: stream/status/anchor advanced across export")
		}
		return ctx.Err()
	}
	if err = check(); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(run, ".snapshot-")
	if err != nil {
		return err
	} // retain failed staging as evidence
	names := []string{"FHIR", "PREP", "FACTS", "OUT"}
	for _, name := range names {
		if _, err = dumpStreamAt(ctx, js, name, filepath.Join(stage, strings.ToLower(name)+"_log.ndjson"), b.Streams[name]); err != nil {
			return err
		}
		if err = check(); err != nil {
			return err
		}
	}
	anchors, err := snapshotAnchors(stage, b)
	if err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	ab, err := json.MarshalIndent(anchors, "", " ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "anchors.json"), ab, 0400); err != nil {
		return err
	}
	res["snapshot_status"], res["snapshot_boundary"] = "complete", b
	rb, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "orchestrate.json"), rb, 0600); err != nil {
		return err
	}
	// Publish only after all original anchors and all cross-stream fences have
	// passed. orchestrate.json is the final success/commit marker, never deferred.
	if err = check(); err != nil {
		return err
	}
	for _, name := range append(names, "anchors", "orchestrate") {
		file := strings.ToLower(name) + "_log.ndjson"
		if name == "anchors" || name == "orchestrate" {
			file = name + ".json"
		}
		if err = os.Rename(filepath.Join(stage, file), filepath.Join(run, file)); err != nil {
			return err
		}
	}
	os.Remove(stage) // empty owned staging directory only
	return nil
}
