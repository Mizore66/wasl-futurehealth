package main

// FHIR polling consumer (ingest tier b) with nightly Bulk Data catch-up (tier d) -> NATS JetStream FHIR stream, for
// an HIE or EMR that offers no Subscription. Publishes exactly what fhirsub publishes (subject fhir.<prov>.res,
// Nats-Msg-Id <prov>/<Type>/<id>/v<vid>, Wsi-Arr = HIE receipt time), so everything downstream is unchanged.
//
//  - poll: GET _history?_since=<watermark - overlap>, paged; a version is new if its id/vid has not been published.
//    The watermark is the newest lastUpdated published; it is checkpointed in a JetStream KV bucket (R3) after the
//    publish acks, so a restart resumes from it, and the seen ledger (seen.go) drops re-published versions.
//  - late commits: a version can become visible after a poll has passed its lastUpdated. The overlap catches short
//    lags; once per simulated day, a $export with _since = watermark - ExportBack catches the rest. Bulk export gives
//    the latest version only, so any skipped intermediate versions are vread (Type/id/_history/vid) and published
//    first, in version order.
//  - end: when the HIE says every version is visible, a final $export without _since runs, then the per-provider EOF.
import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	PollOverlap  = 0.5  // simulated days re-read on each poll
	ExportBack   = 7.0  // simulated days covered by each nightly $export
	PollInterval = 50 * time.Millisecond
	ExportOff    = false // negative control only: no $export catch-up (late commits beyond the overlap are lost)
)

type PollConsumer struct {
	cl        *SubConsumer // reuses req (bearer token) and nothing else
	js        jetstream.JetStream
	kv        jetstream.KeyValue
	seen      *SeenLedger
	runDir    string
	watermark float64            // simulated days
	maxVid    map[string]int     // Type/id -> highest version published
	done      map[string]bool    // Type/id/vid published
	mu        sync.Mutex // guards stats, watermark and eof for statusLoop
	stats     map[string]int64
	lastNight int
	eof       bool
	start     time.Time
}

func runFhirPoll(base, runDir string) {
	nc, js := connect("fhirpoll")
	defer nc.Close()
	ctx := context.Background()
	c := &PollConsumer{cl: &SubConsumer{base: base, http: &http.Client{Timeout: 20 * time.Second}}, js: js, runDir: runDir,
		maxVid: map[string]int{}, done: map[string]bool{}, stats: map[string]int64{}, start: time.Now(), watermark: -1}
	retry("kv", func() (err error) {
		c.kv, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "FHIRPOLL", Replicas: 3, Storage: jetstream.FileStorage})
		return
	})
	c.seen = openSeen(js)
	c.seen.CatchUp(js, "FHIR", 4*seenQueue)
	if e, err := c.kv.Get(ctx, "checkpoint"); err == nil {
		var cp struct {
			W     float64 `json:"w"`
			Night int     `json:"night"`
			Eof   bool    `json:"eof"`
		}
		json.Unmarshal(e.Value(), &cp)
		c.watermark, c.lastNight, c.eof = cp.W, cp.Night, cp.Eof
		c.inc("resumed")
		fmt.Fprintf(os.Stderr, "fhirpoll: resuming from watermark day %.3f\n", cp.W)
	}
	go c.statusLoop()
	for !c.eof {
		complete := c.poll()
		if night := int(c.watermark); night > c.lastNight {
			c.export(c.watermark - ExportBack)
			c.lastNight = night
			c.checkpoint()
		}
		if complete {
			c.export(-1) // final full catch-up, then end of stream
			for _, pc := range Providers {
				m := nats.NewMsg("fhir." + pc.ID + ".ctl")
				m.Data = []byte(`{"eof":true}`)
				m.Header.Set(jetstream.MsgIDHeader, pc.ID+"/eof")
				m.Header.Set("Wsi-Arr", strconv.FormatFloat(TEnd, 'f', 6, 64))
				pubSync(c.js, m)
			}
			c.mu.Lock()
			c.eof = true
			c.mu.Unlock()
			c.checkpoint()
			fmt.Fprintf(os.Stderr, "fhirpoll: feed complete, EOF published (%.1fs)\n", time.Since(c.start).Seconds())
			break
		}
		time.Sleep(PollInterval)
	}
	select {}
}

// poll reads every page since watermark - overlap and publishes new versions; true when the HIE says it is complete.
func (c *PollConsumer) poll() bool {
	since := c.watermark - PollOverlap
	if c.watermark < 0 {
		since = -1000
	}
	complete := false
	for off := 0; ; {
		q := "/_history?_since=" + url.QueryEscape(simInstant(since)) + "&_count=500&_offset=" + strconv.Itoa(off)
		r, _ := http.NewRequest("GET", c.cl.base+q, nil)
		r.Header.Set("Authorization", "Bearer "+MockToken)
		resp, err := c.cl.http.Do(r)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			c.inc("poll_errors")
			return false
		}
		var b Bundle
		json.NewDecoder(resp.Body).Decode(&b)
		resp.Body.Close()
		complete = resp.Header.Get("X-Wsi-Feed-Complete") == "true"
		c.inc("polls")
		maxW := c.watermark
		for _, e := range b.Entry {
			lm := c.watermark
			if e.Response != nil {
				if d, ok := parseInstant(e.Response.LastModified); ok {
					lm = d
				}
			}
			if c.publish(e.Resource, lm, "poll") && lm > maxW {
				maxW = lm
			}
		}
		if maxW > c.watermark {
			c.mu.Lock()
			c.watermark = maxW
			c.mu.Unlock()
			c.checkpoint()
		}
		if len(b.Entry) < 500 {
			break
		}
		off += len(b.Entry)
	}
	return complete
}

// export runs a Bulk Data export (since < 0: everything) and publishes what the polls missed.
func (c *PollConsumer) export(since float64) {
	if ExportOff {
		return
	}
	q := "/$export"
	if since >= 0 {
		q += "?_since=" + url.QueryEscape(simInstant(since))
	}
	r, _ := http.NewRequest("GET", c.cl.base+q, nil)
	r.Header.Set("Authorization", "Bearer "+MockToken)
	resp, err := c.cl.http.Do(r)
	if err != nil || resp.StatusCode != 202 {
		c.inc("export_errors")
		return
	}
	resp.Body.Close()
	b, code, err := c.cl.req("GET", strings.TrimPrefix(resp.Header.Get("Content-Location"), "/fhir"), nil)
	if err != nil || code != 200 {
		c.inc("export_errors")
		return
	}
	var man struct {
		Output []struct{ Type, URL string } `json:"output"`
	}
	json.Unmarshal(b, &man)
	c.inc("exports")
	for _, o := range man.Output {
		fb, code, err := c.cl.req("GET", strings.TrimPrefix(o.URL, "/fhir"), nil)
		if err != nil || code != 200 {
			c.inc("export_errors")
			continue
		}
		sc := bufio.NewScanner(strings.NewReader(string(fb)))
		sc.Buffer(make([]byte, 1<<20), 1<<24)
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			t, id, vid, lm := resourceVersion(line)
			v, _ := strconv.Atoi(vid)
			key := t + "/" + id
			for k := c.maxVid[key] + 1; k < v; k++ { // intermediate versions the polls never saw
				if body, code, err := c.cl.req("GET", "/"+key+"/_history/"+strconv.Itoa(k), nil); err == nil && code == 200 {
					_, _, _, lmk := resourceVersion(body)
					if c.publish(body, lmk, "export_vread") {
						c.inc("export_vread_recovered")
					}
				}
			}
			if c.publish(line, lm, "export") {
				c.inc("export_recovered")
			}
		}
	}
	c.checkpoint()
}

func resourceVersion(body []byte) (typ, id, vid string, lastUpdated float64) {
	var r struct {
		ResourceType, ID string
		Meta             struct{ VersionID, LastUpdated string } `json:"meta"`
	}
	json.Unmarshal(body, &r)
	lu, _ := parseInstant(r.Meta.LastUpdated)
	return r.ResourceType, r.ID, r.Meta.VersionID, lu
}

// publish sends one resource version unless it was already published; true = newly published.
func (c *PollConsumer) publish(body []byte, lastUpdated float64, via string) bool {
	t, id, vid, _ := resourceVersion(body)
	key := t + "/" + id + "/" + vid
	if c.done[key] {
		c.inc("dup_dropped_" + via)
		return false
	}
	var meta struct {
		Meta struct{ Source string } `json:"meta"`
	}
	json.Unmarshal(body, &meta)
	prov := provFromSource(meta.Meta.Source)
	m := nats.NewMsg("fhir." + prov + ".res")
	m.Data = body
	m.Header.Set(jetstream.MsgIDHeader, prov+"/"+t+"/"+id+"/v"+vid)
	m.Header.Set("Wsi-Ots", strconv.FormatInt(time.Now().UnixNano(), 10))
	m.Header.Set("Wsi-Arr", strconv.FormatFloat(lastUpdated, 'f', 6, 64))
	dup, replay := c.seen.Publish(c.js, m)
	c.done[key] = true
	if v, _ := strconv.Atoi(vid); v > c.maxVid[t+"/"+id] {
		c.maxVid[t+"/"+id] = v
	}
	switch {
	case replay:
		c.inc("already_ingested_dropped")
		return false
	case dup:
		c.inc("nats_dup_suppressed")
		return false
	}
	c.inc("published_" + via)
	return true
}

func (c *PollConsumer) inc(k string) {
	c.mu.Lock()
	c.stats[k]++
	c.mu.Unlock()
}

func (c *PollConsumer) checkpoint() {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, _ := json.Marshal(map[string]interface{}{"w": c.watermark, "night": c.lastNight, "eof": c.eof})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	c.kv.Put(ctx, "checkpoint", b)
	cancel()
}

func (c *PollConsumer) statusLoop() {
	for {
		c.mu.Lock()
		s := map[string]int64{}
		keys := make([]string, 0, len(c.stats))
		for k := range c.stats {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s[k] = c.stats[k]
		}
		st := map[string]interface{}{"watermark_day": c.watermark, "eof_sent": c.eof, "done": c.eof, "stats": s, "uptime": time.Since(c.start).Seconds()}
		c.mu.Unlock()
		writeStatus(c.runDir, "fhirpoll", st)
		time.Sleep(300 * time.Millisecond)
	}
}
