package slddoc

import "testing"

func TestParseLayers(t *testing.T) {
	layers, err := parseLayers(`{"layers":[{"id":"2","label":"Мощность реактивная"},{"id":"10","label":"Контейнеры"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 3 {
		t.Fatalf("got %d layers, want 3 (implicit base + 2)", len(layers))
	}
	if layers[0].ID != BaseLayer {
		t.Errorf("layers[0] = %+v, want the implicit base layer first", layers[0])
	}
	if layers[1].ID != 2 || layers[1].Name != "Мощность реактивная" {
		t.Errorf("layers[1] = %+v", layers[1])
	}
}

func TestParseLayers_Empty(t *testing.T) {
	layers, err := parseLayers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 1 || layers[0].ID != BaseLayer {
		t.Errorf("layers = %+v, want just the implicit base layer", layers)
	}
}

func TestResolveLayer(t *testing.T) {
	if resolveLayer("") != BaseLayer {
		t.Error("an empty data-layer must resolve to BaseLayer")
	}
	if resolveLayer("20") != 20 {
		t.Error("a real data-layer must parse through unchanged")
	}
}
