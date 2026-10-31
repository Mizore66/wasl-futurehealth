package main

// wsi: Wasl+Sovereign integration pipeline.
//   wsi gen          -out cohort/                     synthetic multi-provider FHIR cohort + truth + evidence labels
//   wsi orchestrate  -run run/x -speed S -chaos N     3-node NATS JetStream cluster + all services as OS processes (+ kill/restart chaos)
//   wsi audit        -run run/x                       replay FHIR log offline, diff vs live outputs (dups/losses), accuracy, dashboard
//   wsi producer|prep|linker|closure|reviewer|setup   individual services (spawned by orchestrate)
import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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
		fmt.Println("usage: wsi gen|orchestrate|audit|producer|prep|linker|closure|reviewer|setup")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	data := fs.String("data", "../cohort", "cohort dir")
	wasl := fs.String("wasl", "../../wasl/data", "wasl data dir (persons, encoder, scorer)")
	out := fs.String("out", "../cohort", "")
	run := fs.String("run", "../run/default", "run dir")
	prov := fs.String("prov", "", "")
	name := fs.String("name", "0", "")
	part := fs.Int("part", 0, "")
	P := fs.Int("P", 4, "closure partitions")
	threads := fs.Int("threads", 4, "")
	preps := fs.Int("preps", 3, "prep worker processes")
	speed := fs.Float64("speed", 0, "simulated days per wall second (0 = max)")
	chaos := fs.Int("chaos", 0, "kill/restart cycles")
	natsKill := fs.Bool("natskill", false, "also kill+restart one NATS node during the run")
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
	handoff := fs.Bool("handoff", true, "finally rejected referral: hand-off packet to the referrer, act-by deadline and escalation (handoff.go)")
	prepQ := fs.Bool("prepquery", true, "ungraded prep: query the endoscopy unit, escalate at 5 working days, clinician at day 15 (prepquery.go)")
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
	lttOut := fs.String("lttout", "../pipeline/results/ltt/counts.json", "ltt: output")
	split := fs.String("split", "eval", "gen: person split eval (held-out test) | cal (Learn-then-Test calibration)")
	noise := fs.String("noise", "v1", "gen: record-corruption model v1 (training-time) | v2 (independent, held-out evaluation only)")
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
	drData := fs.String("drdata", "", "gen/drcache: diabetic retinopathy corpus dir(s), comma-separated for drcache (default <reports dir>/../dr)")
	encFile := fs.String("encfile", "", "encembed: encoder weights json")
	inFile := fs.String("in", "", "encembed: input file, one name per line")
	modelSet := fs.String("modelset", "", "linkage model chain in the -wasl/-models dir: empty = defaults; X = encoder_X.json + lgb_stream_X.json + iso_X.json + iso_v7_X.json")
	gbmFile := fs.String("gbmfile", "", "featdump: also write the pair-scorer probability from this LightGBM json")
	shadow := fs.Bool("shadow", false, "pilot: shadow mode, every automatic closure is a suggestion confirmed by a clinician (pilot.go)")
	corrShort := fs.Bool("corrshort", true, "linker: a last-surname skeleton under 3 letters corroborates only when the first-name skeleton agrees too (v7.go)")
	reuseConfirm := fs.Bool("reuseconfirm", true, "outreach guard: a second-identifier confirmation counts for the same registration pair on other loops (v7.go)")
	eidSecond := fs.Bool("eidsecond", true, "outreach guard: a matching Emirates ID with agreeing name, sex and DOB counts as the second identifier (v7.go)")
	corrFuzzy := fs.Bool("corrfuzzy", true, "linker: an exact-DOB pair also corroborates when the last surname skeletons (4+ letters) are within one edit (v7.go)")
	corrShared := fs.Bool("corrshared", true, "linker: an exact-DOB pair also corroborates when the names share two distinct 3+ letter skeleton tokens (v7.go)")
	autoAudit := fs.Float64("autoaudit", 0, "pilot: fraction of automatic closures given a post-closure clinician audit (normal mode only; 0 = off)")
	UrduFix = true // retrained v3 encoder was trained with the Urdu/Persian normalisation fix
	fs.Parse(os.Args[2:])
	PrepQuery, ErrUngraded = *prepQ, *errUng
	Handoff, LungWorkup = *handoff, *lungWk
	CorrShared, CorrFuzzyLast, EIDSecondID, CorrShortSurname, ReuseIDConfirm = *corrShared, *corrFuzzy, *eidSecond, *corrShort, *reuseConfirm
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
	ModelSet = *modelSet
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
	PolicyName = policyName()
	if ModelSet != "" {
		PolicyName += "+models=" + ModelSet
	}
	NoiseModel = *noise
	GenSplit = *split
	ReaderMode = *readerMode
	switch *readerMode {
	case "cached":
	case "live", "replay":
		absRunR, _ := filepath.Abs(*run)
		os.MkdirAll(absRunR, 0755)
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
		os.MkdirAll(absRunI, 0755)
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
	v7Args = append(v7Args, fmt.Sprintf("-prepquery=%v", PrepQuery), fmt.Sprintf("-handoff=%v", Handoff), fmt.Sprintf("-lungworkup=%v", LungWorkup), fmt.Sprintf("-corrshared=%v", CorrShared), fmt.Sprintf("-corrfuzzy=%v", CorrFuzzyLast), fmt.Sprintf("-eidsecond=%v", EIDSecondID), fmt.Sprintf("-corrshort=%v", CorrShortSurname), fmt.Sprintf("-reuseconfirm=%v", ReuseIDConfirm), "-errungraded", fmt.Sprint(ErrUngraded))
	if ModelSet != "" {
		v7Args = append(v7Args, "-modelset", ModelSet)
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
		runGen(*wasl, *out, *reports, *pathT, *seed, *loops, *distract, *persons, *cerv, loadPools(*ecPool, *tbPool), *lungN, *lungData, *drN, *drData)
	case "setup":
		setupStreams(*replicas, *P)
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
	case "serve":
		runServe(absRun, *data, *apiAddr)
	case "lungcache": // ask local Gemma 3 4B once per unique lung corpus text; write the replayable cache + reader scores
		runLungCache(*lungData, *readerURL, *out)
	case "drcache": // ask local Gemma 3 4B once per unique DR corpus text; write the replayable cache + reader scores
		runDRCache(*drData, *readerURL, *out)
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
		tamper(*data, *wasl, absRun, *P, *role, *seed)
	case "encembed":
		runEncEmbed(*encFile, *inFile, *out)
	case "encnoise":
		runEncNoise(*wasl, *split, *out, *seed)
	case "featdump":
		runFeatDump(*wasl, *encFile, *gbmFile, *split, *out, *persons, *seed)
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
	Source, Content, Hie, Hook string
	PollSpeed, LagFrac, Overlap float64
	Dup, Reorder, Drop         float64
	SubKill                    int
	HL7Addr                    string
	HL7Drops                   int
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
}

type Orch struct {
	run      string
	bin      string
	procs    map[string]*proc
	order    []string
	mu       sync.Mutex
	stopping bool
}

func (o *Orch) start(name string, bin string, args ...string) {
	o.mu.Lock()
	p, ok := o.procs[name]
	if !ok {
		p = &proc{name: name, args: args}
		o.procs[name] = p
		o.order = append(o.order, name)
		os.MkdirAll(filepath.Join(o.run, "logs"), 0755)
		p.log, _ = os.OpenFile(filepath.Join(o.run, "logs", name+".log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	}
	o.mu.Unlock()
	if bin != "" {
		p.bin = bin
	}
	if p.bin == "" {
		p.bin = o.bin
	}
	o.mu.Lock()
	p.gen++
	gen := p.gen
	cmd := exec.Command(p.bin, p.args...)
	cmd.Stdout, cmd.Stderr = p.log, p.log
	cmd.Dir = filepath.Dir(o.bin)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	p.cmd = cmd
	o.mu.Unlock()
	fmt.Fprintf(p.log, "=== started pid %d at %s\n", cmd.Process.Pid, time.Now().Format(time.RFC3339Nano))
	// supervisor: restart on unexpected exit (restartPolicy: Always), unless killed by chaos or shutting down
	go func() {
		cmd.Wait()
		time.Sleep(300 * time.Millisecond)
		o.mu.Lock()
		restart := !o.stopping && p.gen == gen && !strings.HasPrefix(name, "producer-") && name != "hl7send" // one-shot jobs exit when done
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
	os.MkdirAll(dir, 0755)
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
	os.WriteFile(p, []byte(conf), 0644)
	return p
}

func orchestrate(data, wasl, run string, P, preps, threads int, speed float64, chaos int, natsKill bool, replicas int, noReview bool, seed int64) {
	keepFiles := map[string][]byte{} // keep the live-reader ledgers across reruns (model answers are reused, not re-asked)
	for _, n := range []string{"reader_ledger.jsonl", "image_ledger.jsonl"} {
		if b, _ := os.ReadFile(filepath.Join(run, n)); len(b) > 0 {
			keepFiles[n] = b
		}
	}
	os.RemoveAll(run)
	os.MkdirAll(run, 0755)
	for n, b := range keepFiles {
		os.WriteFile(filepath.Join(run, n), b, 0644)
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
	natsBin := filepath.Join(filepath.Dir(self), "..", "bin", "nats-server")
	for i := 0; i < 3; i++ {
		o.start(fmt.Sprintf("nats-%d", i), natsBin, "-c", natsConf(run, i))
	}
	time.Sleep(1500 * time.Millisecond)
	c := func(args ...string) []string {
		return append(append(args, "-data", data, "-wasl", wasl, "-run", run, "-P", fmt.Sprint(P)), govArgs...)
	}
	setup := exec.Command(self, c("setup", "-replicas", fmt.Sprint(replicas))...)
	setup.Stdout, setup.Stderr = os.Stdout, os.Stderr
	if err := setup.Run(); err != nil {
		panic(err)
	}
	o.start("linker", "", c("linker")...)
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
	victims := [][]string{{"linker"}, {fmt.Sprintf("closure-%d", rng.Intn(P)), "prep-0"}, {"linker", fmt.Sprintf("closure-%d", rng.Intn(P))},
		{fmt.Sprintf("closure-%d", rng.Intn(P)), "prep-1"}, {"linker", "prep-2"}}
	var subDone sync.WaitGroup
	subRng := rand.New(rand.NewSource(seed + 1))
	var evMu sync.Mutex
	subExpect := 6.0 // full-speed feed length (s) measured on this cohort
	if speed > 0 {
		subExpect = expect
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
		if natsKill {
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
	// wait for quiescence
	nc, js := connect("orchestrator")
	defer nc.Close()
	ctx := context.Background()
	stable := 0
	chaosDone := chaos == 0 && !natsKill
	var tQuiet time.Time
	lastSig := ""
	for {
		time.Sleep(500 * time.Millisecond)
		select {
		case <-done:
			chaosDone = true
		default:
		}
		ok := chaosDone
		if SrcCfg.Source == "fhir" {
			if st := readStatus(run, "fhirsub"); st == nil || st["done"] != true {
				ok = false
			}
		} else if SrcCfg.Source == "fhirpoll" {
			if st := readStatus(run, "fhirpoll"); st == nil || st["done"] != true {
				ok = false
			}
		} else if SrcCfg.Source == "hl7" {
			if st := readStatus(run, "hl7send"); st == nil || st["done"] != true {
				ok = false
			}
		} else {
			for _, pc := range Providers {
				st := readStatus(run, "producer-"+pc.ID)
				if st == nil || st["done"] != true {
					ok = false
				}
			}
		}
		fs, err1 := js.Stream(ctx, "FHIR")
		ps, err2 := js.Stream(ctx, "PREP")
		if err1 != nil || err2 != nil {
			continue
		}
		fi, e1 := fs.Info(ctx)
		pi, e2 := ps.Info(ctx)
		if e1 != nil || e2 != nil {
			continue
		}
		ls := readStatus(run, "linker")
		if ls == nil || uint64(ls["processed"].(float64)) != fi.State.Msgs || pi.State.Msgs != fi.State.Msgs {
			ok = false
		}
		sigParts := []string{fmt.Sprint(fi.State.Msgs)}
		if ls == nil || ls["inflight"].(float64) != 0 {
			ok = false
		}
		for k := 0; k < P; k++ {
			cs := readStatus(run, fmt.Sprintf("closure-%d", k))
			if ls == nil || cs == nil || cs["inflight"].(float64) != 0 ||
				uint64(cs["next_pseq"].(float64))-1 != uint64(ls["pseq"].([]interface{})[k].(float64)) {
				ok = false
			} else {
				sigParts = append(sigParts, fmt.Sprint(cs["next_pseq"]))
			}
		}
		if !noReview {
			ost, err := js.Stream(ctx, "OUT")
			if err != nil {
				continue
			}
			oinf, err := ost.Info(ctx)
			rs := readStatus(run, "reviewer")
			if err != nil || rs == nil || uint64(rs["last_out_seq"].(float64)) != oinf.State.LastSeq {
				ok = false
			}
		}
		s := strings.Join(sigParts, ",")
		if ok && s == lastSig {
			stable++
		} else {
			stable = 0
			tQuiet = time.Now()
		}
		lastSig = s
		if stable >= 6 {
			break
		}
		if time.Since(t0) > 5*time.Minute {
			fmt.Println("timeout waiting for quiescence")
			break
		}
	}
	// last OUT publish time = end of processing
	info := func(name string) (i *jetstream.StreamInfo) {
		retry("info "+name, func() error {
			s, err := js.Stream(ctx, name)
			if err != nil {
				return err
			}
			i, err = s.Info(ctx)
			return err
		})
		return
	}
	oi, fi := info("OUT"), info("FHIR")
	lastOut := oi.State.LastTime
	res := map[string]interface{}{"speed": speed, "P": P, "preps": preps, "threads": threads, "replicas": replicas, "chaos": events,
		"fhir_msgs": fi.State.Msgs, "out_msgs": oi.State.Msgs, "t_start": t0, "t_last_out": lastOut, "t_quiet": tQuiet,
		"wall_s": lastOut.Sub(t0).Seconds(), "throughput_ev_s": float64(fi.State.Msgs) / lastOut.Sub(t0).Seconds(),
		"linker": readStatus(run, "linker"), "reviewer": readStatus(run, "reviewer"), "source": SrcCfg,
		"fhirsub": readStatus(run, "fhirsub"), "malaffi": readStatus(run, "malaffi")}
	sup := map[string]interface{}{}
	for n, p := range o.procs {
		sup[n] = map[string]int{"chaos_kills": p.kills, "unexpected_exits_restarted": p.crashes}
	}
	res["supervisor"] = sup
	cl := map[string]interface{}{}
	for k := 0; k < P; k++ {
		cl[fmt.Sprint(k)] = readStatus(run, fmt.Sprintf("closure-%d", k))
	}
	res["closures"] = cl
	anchors := map[string]string{} // v6: ledger heads handed to the auditor (in production: an external, append-only anchor store)
	for k := 0; k < P; k++ {
		if st := readStatus(run, fmt.Sprintf("closure-%d", k)); st != nil && st["ledger_head"] != nil {
			anchors["out.p"+fmt.Sprint(k)] = fmt.Sprint(st["ledger_head"])
		}
	}
	defer func() { // FHIR input anchor: computed over the dumped log, written once (0444) next to the OUT heads
		anchors["fhir"] = fhirAnchor(readLog(filepath.Join(run, "fhir_log.ndjson")))
		ab, _ := json.MarshalIndent(anchors, "", " ")
		os.WriteFile(filepath.Join(run, "anchors.json"), ab, 0444)
	}()
	b, _ := json.MarshalIndent(res, "", " ")
	os.WriteFile(filepath.Join(run, "orchestrate.json"), b, 0644)
	for _, s := range []string{"FHIR", "OUT", "PREP"} {
		n := dumpStream(js, s, filepath.Join(run, strings.ToLower(s)+"_log.ndjson"))
		fmt.Printf("dumped %s: %d msgs\n", s, n)
	}
	fmt.Printf("quiescent: %d FHIR msgs, %d outputs, %.1fs, %.0f ev/s\n", fi.State.Msgs, oi.State.Msgs, res["wall_s"], res["throughput_ev_s"])
	_ = sort.Ints
}

var govArgs []string // v6 reviewer/governance flags forwarded to every child process

type LogMsg struct {
	Seq     uint64            `json:"seq"`
	Subject string            `json:"subj"`
	Hdr     map[string]string `json:"hdr"`
	Data    json.RawMessage   `json:"data"`
	Ts      int64             `json:"ts"`
}

func dumpStream(js jetstream.JetStream, stream, path string) int {
	ctx := context.Background()
	var s jetstream.Stream
	var info *jetstream.StreamInfo
	var cons jetstream.Consumer
	retry("dump "+stream, func() (err error) {
		if s, err = js.Stream(ctx, stream); err != nil {
			return
		}
		if info, err = s.Info(ctx); err != nil {
			return
		}
		cons, err = s.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{})
		return
	})
	f, _ := os.Create(path)
	defer f.Close()
	n := 0
	misses := 0
	for uint64(n) < info.State.Msgs {
		batch, err := cons.Fetch(1000, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			if misses++; misses > 30 {
				panic("dump incomplete: " + stream)
			}
			continue
		}
		got := 0
		for m := range batch.Messages() {
			md, _ := m.Metadata()
			h := map[string]string{}
			for k, v := range m.Headers() {
				h[k] = v[0]
			}
			data := m.Data()
			if !json.Valid(data) {
				data, _ = json.Marshal(string(data))
			}
			b, _ := json.Marshal(LogMsg{Seq: md.Sequence.Stream, Subject: m.Subject(), Hdr: h, Data: data, Ts: md.Timestamp.UnixNano()})
			f.Write(append(b, '\n'))
			n++
			got++
		}
		if got == 0 {
			if misses++; misses > 30 {
				panic("dump incomplete: " + stream)
			}
		}
	}
	return n
}
