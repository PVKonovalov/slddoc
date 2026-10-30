package cim

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PVKonovalov/slddoc/internal/model"
)

// TestCIMHeaders reads the headers of an own export: the layout's names this editor as its authority.
func TestCIMHeaders(t *testing.T) {
	d := &model.Diagram{Elements: []model.Element{{ID: 1, Class: model.ClassBreaker, Shape: "41", Name: "Q", Ports: []model.Port{{Name: "1", Node: 2}, {Name: "2", Node: 3}}}},
		Nodes: []model.Node{{ID: 2}, {ID: 3, Y: 20}}}
	out := ExportCIM(d, testExport(CIMOptions{Name: "folder/Sub", Created: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}))
	headers := CIMHeaders([][]byte{out.EQ, out.DL, []byte(`<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/>`)})
	if len(headers) != 2 {
		t.Fatalf("headers = %+v", headers)
	}
	dl, ok := CIMLayoutHeader(headers)
	isDL := slices.ContainsFunc(dl.Profiles, func(p string) bool { return strings.Contains(p, "DiagramLayout") })
	if !ok || !isDL || dl.AuthoritySet != CIMAuthoritySet || dl.Created != "2026-01-02T03:04:05Z" || dl.Description == "" {
		t.Errorf("layout header = %+v", dl)
	}
	if _, ok := CIMLayoutHeader(nil); ok {
		t.Error("no headers, no layout header")
	}
}
