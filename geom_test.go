package slddoc

import "testing"

func TestParseSubpaths_Leg(t *testing.T) {
	subpaths, err := parseSubpaths("M 900 650 v 3 M 900 670 v -3")
	if err != nil {
		t.Fatal(err)
	}
	if len(subpaths) != 2 {
		t.Fatalf("got %d subpaths, want 2", len(subpaths))
	}
	if got := subpaths[0]; len(got) != 2 || got[0] != (Point{900, 650}) || got[1] != (Point{900, 653}) {
		t.Errorf("subpath 0 = %v", got)
	}
	if got := subpaths[1]; len(got) != 2 || got[0] != (Point{900, 670}) || got[1] != (Point{900, 667}) {
		t.Errorf("subpath 1 = %v", got)
	}
}

func TestParseSubpaths_Box(t *testing.T) {
	subpaths, err := parseSubpaths("M 893 653 h 14 v 14 h -14 z")
	if err != nil {
		t.Fatal(err)
	}
	if len(subpaths) != 1 {
		t.Fatalf("got %d subpaths, want 1", len(subpaths))
	}
	want := []Point{{893, 653}, {907, 653}, {907, 667}, {893, 667}, {893, 653}}
	got := subpaths[0]
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("point %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseSubpaths_Curves(t *testing.T) {
	// Only each curve's endpoint is tracked; control points are consumed.
	// The second path is the real Generator (173) template.
	subpaths, err := parseSubpaths("M 0 0 C 1 1 2 2 3 3 c 1 1 2 2 3 3 S 5 5 10 10 q 1 1 2 2 T 20 20 t 1 0")
	if err != nil {
		t.Fatal(err)
	}
	want := []Point{{0, 0}, {3, 3}, {6, 6}, {10, 10}, {12, 12}, {20, 20}, {21, 20}}
	if len(subpaths) != 1 || len(subpaths[0]) != len(want) {
		t.Fatalf("got %v, want one subpath %v", subpaths, want)
	}
	for i := range want {
		if subpaths[0][i] != want[i] {
			t.Errorf("point %d: got %v, want %v", i, subpaths[0][i], want[i])
		}
	}
	gen, err := parseSubpaths("M 0 0 v 9 m -16 16 a 16 16 0 0 1 32 0 a 16 16 0 0 1 -32 0 M -9 25 c 3 -3 6 -3 9 0 s 6 3 9 0")
	if err != nil {
		t.Fatal(err)
	}
	if last := gen[len(gen)-1]; last[len(last)-1] != (Point{9, 25}) {
		t.Errorf("generator path end: got %v, want {9 25}", last[len(last)-1])
	}
}

func TestParseSubpaths_Arc(t *testing.T) {
	// Only the endpoint is tracked; rx/ry/rotation/flags are consumed but
	// not otherwise interpreted.
	subpaths, err := parseSubpaths("M 0 0 a 5 5 0 1 1 0 10 A 5 5 0 1 1 20 20")
	if err != nil {
		t.Fatal(err)
	}
	if len(subpaths) != 1 {
		t.Fatalf("got %d subpaths, want 1", len(subpaths))
	}
	want := []Point{{0, 0}, {0, 10}, {20, 20}}
	got := subpaths[0]
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("point %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestRotate90(t *testing.T) {
	got := rotate(Point{953, 500}, Point{900, 500}, -270)
	want := Point{900, 553}
	if got != want {
		t.Errorf("rotate(-270) = %v, want %v", got, want)
	}
}

func TestRotateZero(t *testing.T) {
	p := Point{12.5, -3.5}
	if got := rotate(p, Point{0, 0}, 0); got != p {
		t.Errorf("rotate by 0 changed the point: got %v, want %v", got, p)
	}
}

func TestParseSubpaths_TrailingBareCommand(t *testing.T) {
	// A real xsde2svg Power circuit breaker's (399) closed blade.
	got, err := parseSubpaths("M 240 916 v -12 m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0]) != 2 || got[0][1] != (Point{240, 904}) {
		t.Errorf("got %v, want one subpath [(240,916) (240,904)]", got)
	}
}
