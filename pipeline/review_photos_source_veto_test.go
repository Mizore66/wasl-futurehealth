package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// These are metadata-only unit controls, not genuine producer output. The
// unchanged replacement-only and replay controls exercise the actual producer,
// verified loader, unread authenticated image access and immutable OUT bytes.
func digestVetoItem(report string, version int, reader map[string]interface{}) *ReviewItem {
	chain := appendBlock(nil, "evidence", map[string]interface{}{"report": report, "image_reader": reader})
	parts := strings.Split(report, "/")
	return &ReviewItem{
		ReportID: parts[2], ReportVersion: version, Reason: CompletionPurpose,
		Kinds: []string{"evidence"}, Chain: chain, Head: chain[len(chain)-1].Hash,
	}
}

func TestReviewPhotoExplicitProducerDigestConsistencyVeto(t *testing.T) {
	report := "software-a/DiagnosticReport/SAME"
	a := ImgRef{Sha: strings.Repeat("a", 64), Station: "caecum"}
	b := ImgRef{Sha: strings.Repeat("b", 64), Station: "mucosa"}
	c := ImgRef{Sha: strings.Repeat("c", 64), Station: "caecum"}
	for _, tc := range []struct {
		name   string
		reader map[string]interface{}
		refs   []ImgRef
		want   []ImgRef
	}{
		{"matching_native", map[string]interface{}{"photo_shas": []string{a.Sha}}, []ImgRef{a}, []ImgRef{a}},
		{"matching_decoded_reordered_duplicates", map[string]interface{}{"photo_shas": []interface{}{a.Sha, b.Sha, a.Sha}}, []ImgRef{b, a, a}, []ImgRef{a}},
		{"shared_digest_at_two_stations", map[string]interface{}{"photo_shas": []string{a.Sha, a.Sha}}, []ImgRef{a, {Sha: a.Sha, Station: "mucosa"}}, []ImgRef{a}},
		{"replacement_digest", map[string]interface{}{"photo_shas": []string{a.Sha}}, []ImgRef{c}, nil},
		{"missing_original_digest", map[string]interface{}{"photo_shas": []string{a.Sha, b.Sha}}, []ImgRef{a}, nil},
		{"extra_raw_digest", map[string]interface{}{"photo_shas": []string{a.Sha}}, []ImgRef{a, c}, nil},
		{"explicit_empty", map[string]interface{}{"photo_shas": []string{}}, []ImgRef{a}, nil},
		{"explicit_null", map[string]interface{}{"photo_shas": nil}, []ImgRef{a}, nil},
		{"empty_observation", map[string]interface{}{"photo_shas": []string{a.Sha}}, nil, nil},
		{"metadata_absent_path_unchanged", map[string]interface{}{"model": "legacy-software-control"}, []ImgRef{a}, []ImgRef{a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &apiServer{}
			s.observePhotoAssociation(report, 2, tc.refs)
			item := digestVetoItem(report, 2, tc.reader)
			beforeItem, beforeSource := canon(item), canon(s.photos)
			if got := s.itemPhotos(item); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("explicit producer consistency veto: got=%v want=%v", got, tc.want)
			}
			if !bytes.Equal(beforeItem, canon(item)) || !bytes.Equal(beforeSource, canon(s.photos)) {
				t.Fatal("derived consistency check mutated item/head/chain or source index")
			}
		})
	}
}

func TestReviewPhotoProducerDigestVetoIsolationAndAliases(t *testing.T) {
	a := ImgRef{Sha: strings.Repeat("a", 64), Station: "caecum"}
	b := ImgRef{Sha: strings.Repeat("b", 64), Station: "caecum"}
	c := ImgRef{Sha: strings.Repeat("c", 64), Station: "caecum"}
	primary := "software-a/DiagnosticReport/SAME"
	rows := []struct {
		report  string
		version int
		ref     ImgRef
	}{
		{primary, 0, a}, {primary, 1, a}, {primary, 2, b}, {primary, 3, a},
		{"software-a/DiagnosticReport/OTHER", 1, a},
		{"software-b/DiagnosticReport/SAME", 1, a},
	}
	s := &apiServer{}
	items := make([]*ReviewItem, len(rows))
	for i, row := range rows {
		s.observePhotoAssociation(row.report, row.version, []ImgRef{row.ref})
		items[i] = digestVetoItem(row.report, row.version, map[string]interface{}{"photo_shas": []string{row.ref.Sha}})
		if got := s.itemPhotos(items[i]); !reflect.DeepEqual(got, []ImgRef{row.ref}) {
			t.Fatalf("matching anchored source unavailable: %s v%d: %v", row.report, row.version, got)
		}
	}
	// Replace the derived observation rather than append an already-covered
	// conflict. The genuine-producer RED supplies the actual raw-file boundary.
	s.photos[reportVersionKey(primary, 2)] = []ImgRef{c}
	for i, row := range rows {
		got := s.itemPhotos(items[i])
		if row.report == primary && row.version == 2 {
			if len(got) != 0 {
				t.Fatalf("replacement-only digest escaped the explicit original anchor: %v", got)
			}
		} else if !reflect.DeepEqual(got, []ImgRef{row.ref}) {
			t.Fatalf("one source veto crossed provider/report/version or banned a shared digest: %s v%d: %v", row.report, row.version, got)
		}
	}
	s.photos[reportVersionKey(primary, 0)] = []ImgRef{b}
	for _, i := range []int{0, 1} {
		if got := s.itemPhotos(items[i]); len(got) != 0 {
			t.Fatalf("zero/one alias escaped the same original-source veto: %v", got)
		}
	}
	for i := 3; i < len(rows); i++ {
		if got := s.itemPhotos(items[i]); !reflect.DeepEqual(got, []ImgRef{a}) {
			t.Fatalf("legacy alias veto crossed an independent anchored source: %s v%d: %v", rows[i].report, rows[i].version, got)
		}
	}
}

func TestReviewPhotoDigestAnchorRequiresUnambiguousContext(t *testing.T) {
	report := "software-a/DiagnosticReport/SAME"
	digest := strings.Repeat("a", 64)
	for _, name := range []string{"other_item_report", "other_provider", "multiple_evidence", "missing_list", "malformed_list"} {
		t.Run(name, func(t *testing.T) {
			item := digestVetoItem(report, 2, map[string]interface{}{"photo_shas": []string{digest}})
			lookup := report
			switch name {
			case "other_item_report":
				item.ReportID = "OTHER"
			case "other_provider":
				lookup = "software-b/DiagnosticReport/SAME"
			case "multiple_evidence":
				item.Chain = appendBlock(item.Chain, "evidence", map[string]interface{}{"report": report, "image_reader": map[string]interface{}{"photo_shas": []string{digest}}})
			case "missing_list":
				item.Chain = appendBlock(nil, "evidence", map[string]interface{}{"report": report, "image_reader": map[string]interface{}{"model": "legacy-software-control"}})
			case "malformed_list":
				item.Chain = appendBlock(nil, "evidence", map[string]interface{}{"report": report, "image_reader": map[string]interface{}{"photo_shas": []interface{}{3}}})
			}
			if _, explicit := explicitItemPhotoDigests(item, lookup); explicit {
				t.Fatal("unsupported/ambiguous context was treated as an explicit producer source anchor")
			}
		})
	}
}
